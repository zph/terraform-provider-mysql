package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const nonexistingGrantErrCode = 1141

type MySQLGrant struct {
	Database   string
	Table      string
	Privileges []string
	Roles      []string
	Grant      bool
}

func resourceGrant() *schema.Resource {
	return &schema.Resource{
		Create: CreateGrant,
		Update: UpdateGrant,
		Read:   ReadGrant,
		Delete: DeleteGrant,
		Importer: &schema.ResourceImporter{
			State: ImportGrant,
		},

		Schema: map[string]*schema.Schema{
			"user": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"role"},
			},

			"role": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"user", "host"},
			},

			"host": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				Default:       "localhost",
				ConflictsWith: []string{"role"},
			},

			"database": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"table": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "*",
			},

			"privileges": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},

			"roles": {
				Type:          schema.TypeSet,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"privileges"},
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
			},

			"grant": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
			},

			"tls_option": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "NONE",
			},
		},
	}
}

func flattenList(list []interface{}, template string) string {
	var result []string
	for _, v := range list {
		result = append(result, fmt.Sprintf(template, v.(string)))
	}

	return strings.Join(result, ", ")
}

func formatDatabaseName(database string) string {
	if strings.Compare(database, "*") != 0 && !strings.HasSuffix(database, "`") {
		return fmt.Sprintf("`%s`", database)
	}

	return database
}

func formatTableName(table string) string {
	if table == "" || table == "*" {
		return fmt.Sprintf("*")
	}
	return fmt.Sprintf("`%s`", table)
}

func userOrRole(user string, host string, role string, hasRoles bool) (string, bool, error) {
	if len(user) > 0 && len(host) > 0 {
		return fmt.Sprintf("'%s'@'%s'", user, host), false, nil
	} else if len(role) > 0 {
		if !hasRoles {
			return "", false, fmt.Errorf("Roles are only supported on MySQL 8 and above")
		}

		return fmt.Sprintf("'%s'", role), true, nil
	} else {
		return "", false, fmt.Errorf("user with host or a role is required")
	}
}

func supportsRoles(db *sql.DB) (bool, error) {
	currentVersion, err := serverVersion(db)
	if err != nil {
		return false, err
	}

	requiredVersion, _ := version.NewVersion("8.0.0")
	hasRoles := currentVersion.GreaterThan(requiredVersion)
	return hasRoles, nil
}

func CreateGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}

	var (
		privilegesOrRoles string
		grantOn           string
	)

	hasPrivs := false
	rolesGranted := 0
	if attr, ok := d.GetOk("privileges"); ok {
		privilegesOrRoles = flattenList(attr.(*schema.Set).List(), "%s")
		hasPrivs = true
	} else if attr, ok := d.GetOk("roles"); ok {
		if !hasRoles {
			return fmt.Errorf("Roles are only supported on MySQL 8 and above")
		}
		listOfRoles := attr.(*schema.Set).List()
		rolesGranted = len(listOfRoles)
		privilegesOrRoles = flattenList(listOfRoles, "'%s'")
	} else {
		return fmt.Errorf("One of privileges or roles is required")
	}

	user := d.Get("user").(string)
	host := d.Get("host").(string)
	role := d.Get("role").(string)

	userOrRole, isRole, err := userOrRole(user, host, role, hasRoles)
	if err != nil {
		return err
	}

	grants, err := showGrants(db, userOrRole)
	for _, grant := range grants {
		if hasPrivs {
			if grant.Database == d.Get("database").(string) && grant.Table == d.Get("table").(string) {
				return fmt.Errorf("user/role %s already has unmanaged grant to %s.%s - import it first", userOrRole, grant.Database, grant.Table)
			}
		} else {
			// Granting role is just role without DB & table.
			if grant.Database == "" && grant.Table == "" {
				return fmt.Errorf("user/role %s already has unmanaged grant for roles %v - import it first", userOrRole, grant.Roles)
			}
		}
	}

	database := formatDatabaseName(d.Get("database").(string))

	table := formatTableName(d.Get("table").(string))

	if (!isRole || hasPrivs) && rolesGranted == 0 {
		grantOn = fmt.Sprintf(" ON %s.%s", database, table)
	}

	stmtSQL := fmt.Sprintf("GRANT %s%s TO %s",
		privilegesOrRoles,
		grantOn,
		userOrRole)

	// MySQL 8+ doesn't allow REQUIRE on a GRANT statement.
	if !hasRoles && d.Get("tls_option").(string) != "" {
		stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
	}

	if !hasRoles && !isRole && d.Get("grant").(bool) {
		stmtSQL += " WITH GRANT OPTION"
	}

	log.Println("Executing statement:", stmtSQL)
	_, err = db.Exec(stmtSQL)
	if err != nil {
		return fmt.Errorf("Error running SQL (%s): %s", stmtSQL, err)
	}

	id := fmt.Sprintf("%s@%s:%s", user, host, database)
	if isRole {
		id = fmt.Sprintf("%s:%s", role, database)
	}

	d.SetId(id)

	return ReadGrant(d, meta)
}

func ReadGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}

	userOrRole, _, err := userOrRole(
		d.Get("user").(string),
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)
	if err != nil {
		return err
	}

	grants, err := showGrants(db, userOrRole)

	if err != nil {
		log.Printf("[WARN] GRANT not found for %s (%s) - removing from state", userOrRole, err)
		d.SetId("")
		return nil
	}
	database := d.Get("database").(string)
	table := d.Get("table").(string)

	var privileges []string
	var roles []string
	var grantOption bool

	for _, grant := range grants {
		if grant.Database == database && grant.Table == table {
			privileges = makePrivs(setToArray(d.Get("privileges")), grant.Privileges)
		}
		// Granting role is just role without DB & table.
		if grant.Database == "" && grant.Table == "" {
			roles = grant.Roles
		}

		if grant.Grant {
			grantOption = true
		}
	}

	d.Set("privileges", privileges)
	d.Set("roles", roles)
	d.Set("grant", grantOption)

	return nil
}

func UpdateGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	hasRoles, err := supportsRoles(db)

	if err != nil {
		return err
	}

	userOrRole, _, err := userOrRole(
		d.Get("user").(string),
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)

	if err != nil {
		return err
	}

	database := d.Get("database").(string)
	table := d.Get("table").(string)

	if d.HasChange("privileges") {
		err = updatePrivileges(d, db, userOrRole, database, table)

		if err != nil {
			return err
		}
	}

	return nil
}

func updatePrivileges(d *schema.ResourceData, db *sql.DB, user string, database string, table string) error {
	oldPrivsIf, newPrivsIf := d.GetChange("privileges")
	oldPrivs := oldPrivsIf.(*schema.Set)
	newPrivs := newPrivsIf.(*schema.Set)
	grantIfs := newPrivs.Difference(oldPrivs).List()
	revokeIfs := oldPrivs.Difference(newPrivs).List()

	if len(revokeIfs) > 0 {
		revokes := make([]string, len(revokeIfs))

		for i, v := range revokeIfs {
			revokes[i] = v.(string)
		}

		sql := fmt.Sprintf("REVOKE %s ON %s.%s FROM %s", strings.Join(revokes, ","), formatDatabaseName(database), formatTableName(table), user)

		log.Printf("[DEBUG] SQL: %s", sql)

		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	if len(grantIfs) > 0 {
		grants := make([]string, len(grantIfs))

		for i, v := range grantIfs {
			grants[i] = v.(string)
		}

		sql := fmt.Sprintf("GRANT %s ON %s.%s TO %s", strings.Join(grants, ","), formatDatabaseName(database), formatTableName(table), user)

		log.Printf("[DEBUG] SQL: %s", sql)

		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	return nil
}

func DeleteGrant(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	database := formatDatabaseName(d.Get("database").(string))

	table := formatTableName(d.Get("table").(string))

	hasRoles, err := supportsRoles(db)
	if err != nil {
		return err
	}

	userOrRole, isRole, err := userOrRole(
		d.Get("user").(string),
		d.Get("host").(string),
		d.Get("role").(string),
		hasRoles)
	if err != nil {
		return err
	}

	roles := d.Get("roles").(*schema.Set)
	privileges := d.Get("privileges").(*schema.Set)

	var sql string
	if !isRole && len(roles.List()) == 0 {
		sql = fmt.Sprintf("REVOKE GRANT OPTION ON %s.%s FROM %s",
			database,
			table,
			userOrRole)

		log.Printf("[DEBUG] SQL: %s", sql)
		_, err = db.Exec(sql)
		if err != nil {
			if !isNonExistingGrant(err) {
				return fmt.Errorf("error revoking GRANT (%s): %s", sql, err)
			}
		}
	}

	whatToRevoke := fmt.Sprintf("ALL ON %s.%s", database, table)
	if len(roles.List()) > 0 {
		whatToRevoke = flattenList(roles.List(), "'%s'")
	} else if len(privileges.List()) > 0 {
		privilegeList := flattenList(privileges.List(), "%s")
		whatToRevoke = fmt.Sprintf("%s ON %s.%s", privilegeList, database, table)
	}

	sql = fmt.Sprintf("REVOKE %s FROM %s", whatToRevoke, userOrRole)
	log.Printf("[DEBUG] SQL: %s", sql)
	_, err = db.Exec(sql)
	if err != nil {
		if !isNonExistingGrant(err) {
			return fmt.Errorf("error revoking ALL (%s): %s", sql, err)
		}
	}

	return nil
}

func isNonExistingGrant(err error) bool {
	if driverErr, ok := err.(*mysql.MySQLError); ok {
		if driverErr.Number == nonexistingGrantErrCode {
			return true
		}
	}
	return false
}

func ImportGrant(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	userHost := strings.SplitN(d.Id(), "@", 2)

	if len(userHost) != 2 {
		return nil, fmt.Errorf("wrong ID format %s (expected USER@HOST)", d.Id())
	}

	user := userHost[0]
	host := userHost[1]

	db := meta.(*MySQLConfiguration).Db

	grants, err := showGrants(db, fmt.Sprintf("'%s'@'%s'", user, host))

	if err != nil {
		return nil, err
	}

	results := []*schema.ResourceData{}

	for _, grant := range grants {
		results = append(results, restoreGrant(user, host, grant))
	}

	return results, nil
}

func restoreGrant(user string, host string, grant *MySQLGrant) *schema.ResourceData {
	d := resourceGrant().Data(nil)

	database := grant.Database
	id := fmt.Sprintf("%s@%s:%s", user, host, formatDatabaseName(database))
	d.SetId(id)

	d.Set("user", user)
	d.Set("host", host)
	d.Set("database", database)
	d.Set("table", grant.Table)
	d.Set("grant", grant.Grant)
	d.Set("tls_option", "NONE")
	d.Set("privileges", grant.Privileges)

	return d
}

func showGrants(db *sql.DB, user string) ([]*MySQLGrant, error) {
	grants := []*MySQLGrant{}

	sql := fmt.Sprintf("SHOW GRANTS FOR %s", user)
	rows, err := db.Query(sql)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	re := regexp.MustCompile(`^GRANT (.+) ON (.+?)\.(.+?) TO`)

	// Ex: GRANT `app_read`@`%`,`app_write`@`%` TO `rw_user1`@`localhost
	reRole := regexp.MustCompile(`^GRANT (.+) TO`)
	reGrant := regexp.MustCompile(`\bGRANT OPTION\b`)

	for rows.Next() {
		var rawGrant string

		err := rows.Scan(&rawGrant)
		if err != nil {
			return nil, err
		}

		if m := re.FindStringSubmatch(rawGrant); len(m) == 4 {
			privsStr := m[1]
			priv_list := extractPermTypes(privsStr)
			privileges := make([]string, len(priv_list))

			for i, priv := range priv_list {
				privileges[i] = strings.TrimSpace(priv)
			}

			grant := &MySQLGrant{
				Database:   strings.ReplaceAll(m[2], "`", ""),
				Table:      strings.Trim(m[3], "`"),
				Privileges: privileges,
				Grant:      reGrant.MatchString(rawGrant),
			}

			grants = append(grants, grant)

		} else if m := reRole.FindStringSubmatch(rawGrant); len(m) == 2 {
			roleStr := m[1]
			rolesStart := strings.Split(roleStr, ",")
			roles := make([]string, len(rolesStart))

			for i, role := range rolesStart {
				roles[i] = strings.Trim(role, "`@% ")
			}

			grant := &MySQLGrant{
				Roles: roles,
				Grant: reGrant.MatchString(rawGrant),
			}

			grants = append(grants, grant)
		} else {
			return nil, fmt.Errorf("failed to parse grant statement: %s", rawGrant)
		}
	}

	return grants, nil
}

func normalizeColumnOrderMulti(perm []string) []string {
	ret := []string{}
	for _, p := range perm {
		ret = append(ret, normalizeColumnOrder(p))
	}
	return ret
}

func extractPermTypes(g string) []string {
	grants := []string{}

	inParentheses := false
	currentWord := []rune{}
	for _, b := range g {
		switch b {
		case ',':
			if inParentheses {
				currentWord = append(currentWord, b)
			} else {
				grants = append(grants, string(currentWord))
				currentWord = []rune{}
			}
			break
		case '(':
			inParentheses = true
			currentWord = append(currentWord, b)
			break
		case ')':
			inParentheses = false
			currentWord = append(currentWord, b)
			break
		default:
			if unicode.IsSpace(b) && len(currentWord) == 0 {
				break
			}
			currentWord = append(currentWord, b)
		}
	}
	grants = append(grants, string(currentWord))
	return grants
}

func normalizeColumnOrder(perm string) string {
	re := regexp.MustCompile("^([^(]*)\\((.*)\\)$")
	// We may get inputs like
	// 	SELECT(b,a,c)   -> SELECT(a,b,c)
	// 	DELETE          -> DELETE
	// if it's without parentheses, return it right away.
	// Else split what is inside, sort it, concat together and return the result.
	m := re.FindStringSubmatch(perm)
	if m == nil || len(m) < 3 {
		return perm
	}

	parts := strings.Split(m[2], ",")
	for i := range parts {
		parts[i] = strings.Trim(parts[i], "` ")
	}
	sort.Strings(parts)
	partsTogether := strings.Join(parts, ", ")
	return fmt.Sprintf("%s(%s)", m[1], partsTogether)
}

func normalizePerms(perms []string) []string {
	// Spaces and backticks are optional, let's ignore them.
	re := regexp.MustCompile("[ `]")
	ret := []string{}
	for _, perm := range perms {
		permNorm := re.ReplaceAllString(perm, "")
		permUcase := strings.ToUpper(permNorm)
		if permUcase == "ALL" || permUcase == "ALLPRIVILEGES" {
			permUcase = "ALL PRIVILEGES"
		}
		permSortedColumns := normalizeColumnOrder(permUcase)
		ret = append(ret, permSortedColumns)
	}
	return ret
}

func makePrivs(have, want []string) []string {
	// This is tricky to prevent diffs that cannot be suppressed easily.
	// Example:
	// Have select(`c1`, `c2`), insert (c3,c2)
	// Want select(c2,c1), insert(c3,c2)
	// So we want to normalize both and then go from "want" to "have" to
	// We'll have map want->wantnorm = havenorm->have

	// Also, we need to return all mapped values of "want".

	// After normalize, same indices have the same values, prepare maps.
	haveNorm := normalizePerms(have)
	haveNormToHave := map[string]string{}
	for i := range haveNorm {
		haveNormToHave[haveNorm[i]] = have[i]
	}

	wantNorm := normalizePerms(want)
	wantNormToWant := map[string]string{}
	for i := range wantNorm {
		wantNormToWant[want[i]] = wantNorm[i]
	}

	retSet := []string{}
	for _, w := range want {
		suspect := haveNormToHave[wantNormToWant[w]]
		if suspect == "" {
			// Nothing found in what we have.
			retSet = append(retSet, w)
		} else {
			retSet = append(retSet, suspect)
		}
	}

	return retSet
}

func setToArray(s interface{}) []string {
	set, ok := s.(*schema.Set)
	if !ok {
		return []string{}
	}

	ret := []string{}
	for _, elem := range set.List() {
		ret = append(ret, elem.(string))
	}
	return ret
}
