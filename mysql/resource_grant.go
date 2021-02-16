package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

const nonexistingGrantErrCode = 1141

func resourceGrant() *schema.Resource {
	return &schema.Resource{
		Create: CreateGrant,
		Update: nil,
		Read:   ReadGrant,
		Delete: DeleteGrant,

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
				ForceNew: true,
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
	db, err := connectToMySQL(meta.(*MySQLConfiguration))
	if err != nil {
		return err
	}

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
	db, err := connectToMySQL(meta.(*MySQLConfiguration))
	if err != nil {
		return err
	}

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

	sql := fmt.Sprintf("SHOW GRANTS FOR %s", userOrRole)

	log.Println("[DEBUG] SQL:", sql)

	rows, err := db.Query(sql)
	if err != nil {
		log.Printf("[WARN] GRANT not found for %s - removing from state", userOrRole)
		d.SetId("")
	}
	defer rows.Close()

	for rows.Next() {
		grant := ""
		if err := rows.Scan(&grant); err != nil {
			log.Fatal(err)
		}

		hasThem, err := hasPermissions(grant,
			Perms{
				User:     d.Get("user").(string),
				Host:     d.Get("host").(string),
				DB:       d.Get("database").(string),
				Table:    d.Get("table").(string),
				PermType: d.Get("privileges").([]string),
			})
		if err != nil {
			log.Printf("[WARN] Getting perms failed with %s", err)
			return nil
		}
		if hasThem {
			return nil
		}
	}

	// Nothing was found.
	log.Println("[DEBUG] No grants were found.")
	d.SetId("")
	return nil
}

func DeleteGrant(d *schema.ResourceData, meta interface{}) error {
	db, err := connectToMySQL(meta.(*MySQLConfiguration))
	if err != nil {
		return err
	}

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

type Perms struct {
	User     string
	Host     string
	DB       string
	Table    string
	PermType []string
}

func parsePermTypes(permTypes string) []string {
	return normalizePerms(extractPermTypes(permTypes))
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


func normalizePerms(perms []string) []string {
	// Spaces and backticks are optional, let's ignore them.
	re := regexp.MustCompile("[ `]")
	ret := []string{}
	for _, perm := range perms {
		permNorm := re.ReplaceAllString(perm, "")
		permUcase := strings.ToUpper(permNorm)
		if permUcase == "ALL" || permUcase == "ALLPERMISSIONS" {
			permUcase = "ALL PERMISSIONS"
		}
		ret = append(ret, permUcase)
	}
	return ret
}

func parseGrants(grants string) (Perms, error) {
	// We don't support roles here. Yes, we should have a parser here. Shame on me.
	re := regexp.MustCompile("^GRANT +(.*) +ON +`?([^`. ]*)`?[.]`?([^`. ]*)`? +TO +'([^']*)'@'([^']*)'")
	m := re.FindStringSubmatch(grants)
	if m == nil || len(m) < 6 {
		return Perms{}, fmt.Errorf("failed parsing grants, maybe you are using roles? Grants: %s, m: %v", grants, m)
	}
	return Perms{
		User:     m[4],
		Host:     m[5],
		DB:       m[2],
		Table:    m[3],
		PermType: parsePermTypes(m[1]),
	}, nil
}

func hasPermissions(grants string, sourcePerms Perms) (bool, error) {
	realPerms, err := parseGrants(grants)
	if err != nil {
		return false, err
	}

	if sourcePerms.User != realPerms.User || sourcePerms.Host != realPerms.Host ||
		sourcePerms.DB != realPerms.DB || sourcePerms.Table != realPerms.Table {
		log.Printf("[DEBUG] Not matching user/db/host - in config %+v, in reality %+v\n", sourcePerms, realPerms)
		return false, nil
	}
	sourcePerms.PermType = normalizePerms(sourcePerms.PermType)

	log.Printf("[DEBUG] Matching user/db/host to check - in config %+v, in reality %+v\n", sourcePerms, realPerms)
	for _, wantPerm := range sourcePerms.PermType {
		havePerm := false
		// n^2, but it's more effective than sorting as there are at most 5 items.
		for _, realPerm := range realPerms.PermType {
			if wantPerm == realPerm {
				havePerm = true
			}
		}
		if !havePerm {
			// fmt.Printf("Missing perm %s", wantPerm)
			return false, nil
		}
	}
	return true, nil
}

