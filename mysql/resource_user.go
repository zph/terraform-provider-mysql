package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateUser,
		UpdateContext: UpdateUser,
		ReadContext:   ReadUser,
		DeleteContext: DeleteUser,
		Importer: &schema.ResourceImporter{
			StateContext: ImportUser,
		},
		CustomizeDiff: func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			if _, ok := d.GetOk("max_user_connections"); ok {
				if err := checkMaxUserConnectionsSupport(ctx, meta); err != nil {
					return err
				}
			}
			if _, ok := d.GetOk("max_statement_time"); ok {
				if err := checkMaxStatementTimeSupport(ctx, meta); err != nil {
					return err
				}
			}
			return nil
		},

		Schema: map[string]*schema.Schema{
			"user": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"host": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "localhost",
			},

			"plaintext_password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				StateFunc: hashSum,
			},

			"password": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"plaintext_password"},
				Sensitive:     true,
				Deprecated:    "Please use plaintext_password instead",
			},

			"auth_plugin": {
				Type:             schema.TypeString,
				Optional:         true,
				ForceNew:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
				ConflictsWith:    []string{"plaintext_password", "password"},
			},

			"aad_identity": {
				Type:     schema.TypeSet,
				Optional: true,
				ForceNew: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Default:  "user",
							ValidateFunc: validation.StringInSlice([]string{
								"user",
								"group",
								"service_principal",
							}, false),
						},
						"identity": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
					},
				},
			},

			"auth_string_hashed": {
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
				ConflictsWith:    []string{"plaintext_password", "password"},
			},

			"tls_option": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "NONE",
			},

			"retain_old_password": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			// TiDB supports binding users to resource groups directly in CREATE/ALTER USER.
			// See https://docs.pingcap.com/tidb/stable/sql-statement-create-user/.
			"resource_group": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"max_user_connections": {
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntAtLeast(0),
				Description:  "Maximum number of simultaneous connections for the user (0 = unlimited). Read back for drift detection on MySQL and MariaDB. On TiDB the clause is accepted only on 8.5.5 or newer; readback engages when the build exposes the mysql.user.max_user_connections column (detected at runtime, since not all 8.5.x builds carry it), otherwise it is applied best-effort and write-only.",
			},

			"max_statement_time": {
				Type:         schema.TypeFloat,
				Optional:     true,
				ValidateFunc: validation.FloatAtLeast(0),
				Description:  "Maximum execution time for statements in seconds (0 = unlimited). Supports fractional values. Only supported on MariaDB 10.1.1 or newer.",
			},

			// TiDB CREATE/ALTER USER supports account locking, comments, JSON
			// attributes, and password lifecycle options.
			// See https://docs.pingcap.com/tidb/stable/sql-statement-alter-user/.
			"account_locked": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},

			"comment": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"attribute_json": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateFunc:     validation.StringIsJSON,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"password_expire": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"password_history": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"password_reuse_interval": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},

			"failed_login_attempts": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},

			"password_lock_time": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
			},
		},
	}
}

const tiDBMaxUserConnectionsMinVersion = "8.5.5"

func checkRetainCurrentPasswordSupport(ctx context.Context, meta interface{}) error {
	ver, _ := version.NewVersion("8.0.14")
	if getVersionFromMeta(ctx, meta).LessThan(ver) {
		return errors.New("MySQL version must be at least 8.0.14")
	}
	return nil
}

func serverMariaDB(db *sql.DB) (bool, error) {
	versionString, err := serverVersionString(db)
	if err != nil {
		return false, err
	}

	return strings.Contains(versionString, "MariaDB"), nil
}

func tidbVersionSupportsMaxUserConnections(tidbVersion string) (bool, error) {
	currentVersion, err := version.NewVersion(strings.TrimPrefix(tidbVersion, "v"))
	if err != nil {
		return false, err
	}
	minVersion, _ := version.NewVersion(tiDBMaxUserConnectionsMinVersion)

	return currentVersion.GreaterThanOrEqual(minVersion), nil
}

// tidbUserTableHasMaxUserConnections reports whether the running TiDB build
// exposes the mysql.user.max_user_connections column. TiDB does not echo the
// resource-limit clause in SHOW CREATE USER, and column presence does not
// track the version string: builds carrying pingcap/tidb#59197 (e.g. custom
// 8.5.5 builds) have the column, while the upstream v8.5.x Docker images at
// bootstrap version 227 do not. Probe information_schema rather than gating on
// the version so readback engages exactly on the builds that support it.
func tidbUserTableHasMaxUserConnections(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	const stmt = "SELECT COUNT(*) FROM information_schema.columns WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'user' AND LOWER(COLUMN_NAME) = 'max_user_connections'"
	if err := db.QueryRowContext(ctx, stmt).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// readTiDBMaxUserConnections refreshes max_user_connections from the mysql.user
// column when the running TiDB build exposes it, giving a true readback path
// (0 = unlimited) for drift detection. On builds without the column, and on
// non-TiDB servers, it is a no-op and the value stays as last applied.
func readTiDBMaxUserConnections(ctx context.Context, db *sql.DB, d *schema.ResourceData) error {
	isTiDB, _, _, err := serverTiDB(db)
	if err != nil {
		return err
	}
	if !isTiDB {
		return nil
	}

	hasColumn, err := tidbUserTableHasMaxUserConnections(ctx, db)
	if err != nil {
		return err
	}
	if !hasColumn {
		return nil
	}

	var maxUserConn int
	const stmt = "SELECT max_user_connections FROM mysql.user WHERE User = ? AND Host = ?"
	if err := db.QueryRowContext(ctx, stmt, d.Get("user").(string), d.Get("host").(string)).Scan(&maxUserConn); err != nil {
		return err
	}

	return d.Set("max_user_connections", maxUserConn)
}

func checkMaxUserConnectionsSupport(ctx context.Context, meta interface{}) error {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return err
	}

	isTiDB, tidbVersion, _, err := serverTiDB(db)
	if err != nil {
		return err
	}
	if !isTiDB {
		return nil
	}

	// TiDB added MAX_USER_CONNECTIONS on master in pingcap/tidb#59197.
	// The first explicit 8.5 release branch backport is pingcap/tidb#67337,
	// merged as commit 6c7aaa0c8d548cdfaa2c99e216337752de48009f to
	// release-8.5-20260323-v8.5.5.
	supported, err := tidbVersionSupportsMaxUserConnections(tidbVersion)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("MAX_USER_CONNECTIONS is only supported on TiDB %s or newer", tiDBMaxUserConnectionsMinVersion)
	}

	return nil
}

func redactCreateUserArgs(args []interface{}, sensitiveValues ...string) []interface{} {
	redacted := make([]interface{}, len(args))
	copy(redacted, args)

	for i, arg := range redacted {
		argString, ok := arg.(string)
		if !ok {
			continue
		}
		for _, sensitive := range sensitiveValues {
			if sensitive != "" && argString == sensitive {
				redacted[i] = "<SENSITIVE>"
				break
			}
		}
	}

	return redacted
}

func checkMaxStatementTimeSupport(ctx context.Context, meta interface{}) error {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return err
	}

	isMariaDB, err := serverMariaDB(db)
	if err != nil {
		return err
	}
	if !isMariaDB {
		return errors.New("MAX_STATEMENT_TIME is only supported on MariaDB 10.1.1 or newer")
	}

	minVersion, _ := version.NewVersion("10.1.1")
	currentVersion := getVersionFromMeta(ctx, meta)
	if currentVersion.LessThan(minVersion) {
		return fmt.Errorf("MAX_STATEMENT_TIME requires MariaDB 10.1.1 or newer (current version: %s)", currentVersion.String())
	}

	return nil
}

func CreateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	var authStm string
	var auth string
	var createObj = "USER"
	user := d.Get("user").(string)
	host := d.Get("host").(string)

	if v, ok := d.GetOk("auth_plugin"); ok {
		auth = v.(string)
	}

	if len(auth) > 0 {
		if auth == "aad_auth" {
			// aad_auth is plugin but Microsoft uses another statement to create this kind of users
			createObj = "AADUSER"
			if _, ok := d.GetOk("aad_identity"); !ok {
				return diag.Errorf("aad_identity is required for aad_auth")
			}
		} else if auth == "AWSAuthenticationPlugin" {
			authStm = " IDENTIFIED WITH AWSAuthenticationPlugin as 'RDS'"
		} else {
			// mysql_no_login, auth_pam, ...
			authStm = " IDENTIFIED WITH " + auth
		}
	}
	var hashed string
	if v, ok := d.GetOk("auth_string_hashed"); ok {
		hashed = v.(string)
		if hashed != "" {
			if authStm == "" {
				return diag.Errorf("auth_string_hashed is not supported for auth plugin %s", auth)
			}
			authStm = fmt.Sprintf("%s AS ?", authStm)
		}
	}

	var stmtSQL string
	var args []interface{}

	if createObj == "AADUSER" {
		var aadIdentity = d.Get("aad_identity").(*schema.Set).List()[0].(map[string]interface{})

		if aadIdentity["type"].(string) == "service_principal" {
			// CREATE AADUSER 'mysqlProtocolLoginName"@"mysqlHostRestriction' IDENTIFIED BY 'identityId'
			stmtSQL = "CREATE AADUSER ?@? IDENTIFIED BY ?"
			args = []interface{}{user, host, aadIdentity["identity"].(string)}
		} else {
			// CREATE AADUSER 'identityName"@"mysqlHostRestriction' AS 'mysqlProtocolLoginName'
			stmtSQL = "CREATE AADUSER ?@? AS ?"
			args = []interface{}{aadIdentity["identity"].(string), host, user}
		}
	} else {
		stmtSQL = "CREATE USER ?@?"
		args = []interface{}{user, host}
	}

	var password string
	if v, ok := d.GetOk("plaintext_password"); ok {
		password = v.(string)
	} else {
		password = d.Get("password").(string)
	}

	if auth == "AWSAuthenticationPlugin" && host == "localhost" {
		return diag.Errorf("cannot use IAM auth against localhost")
	}

	if authStm != "" {
		stmtSQL = stmtSQL + authStm
		if hashed != "" {
			args = append(args, hashed)
		}
		if password != "" {
			stmtSQL = stmtSQL + " BY ?"
			args = append(args, password)
		}
	} else if password != "" {
		stmtSQL = stmtSQL + " IDENTIFIED BY ?"
		args = append(args, password)
	}

	requiredVersion, _ := version.NewVersion("5.7.0")

	var updateStmtSql = ""

	if getVersionFromMeta(ctx, meta).GreaterThan(requiredVersion) && d.Get("tls_option").(string) != "" {
		if createObj == "AADUSER" {
			updateStmtSql = "ALTER USER ?@? REQUIRE " + d.Get("tls_option").(string)
		} else {
			stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
		}
	}

	var resourceLimits []string
	if createObj != "AADUSER" {
		if maxConn, ok := d.GetOk("max_user_connections"); ok {
			if err := checkMaxUserConnectionsSupport(ctx, meta); err != nil {
				return diag.FromErr(err)
			}
			resourceLimits = append(resourceLimits, fmt.Sprintf("MAX_USER_CONNECTIONS %d", maxConn.(int)))
		}

		if maxStmt, ok := d.GetOk("max_statement_time"); ok {
			if err := checkMaxStatementTimeSupport(ctx, meta); err != nil {
				return diag.FromErr(err)
			}
			resourceLimits = append(resourceLimits, fmt.Sprintf("MAX_STATEMENT_TIME %f", maxStmt.(float64)))
		}

		createUserWithVersion, _ := version.NewVersion("5.7.6")
		if len(resourceLimits) > 0 && getVersionFromMeta(ctx, meta).GreaterThanOrEqual(createUserWithVersion) {
			stmtSQL += " WITH " + strings.Join(resourceLimits, " ")
		}
	}

	retainPassword := d.Get("retain_old_password").(bool)
	if retainPassword {
		err := checkRetainCurrentPasswordSupport(ctx, meta)
		if err != nil {
			return diag.Errorf("cannot use retain_current_password: %v", err)
		}
	}

	if createObj == "USER" {
		stmtSQL = appendTiDBUserOptionClauses(stmtSQL, buildTiDBUserOptionClauses(d, false))
	}

	log.Println("[DEBUG] Executing statement:", stmtSQL, "args:", redactCreateUserArgs(args, password, hashed))
	_, err = db.ExecContext(ctx, stmtSQL, args...)
	if err != nil {
		return diag.Errorf("failed executing SQL: %v", err)
	}

	createUserWithVersion, _ := version.NewVersion("5.7.6")
	if createObj != "AADUSER" && len(resourceLimits) > 0 && getVersionFromMeta(ctx, meta).LessThan(createUserWithVersion) {
		grantStmtSQL := "GRANT USAGE ON *.* TO ?@? WITH " + strings.Join(resourceLimits, " ")

		log.Println("[DEBUG] Executing statement:", grantStmtSQL, "args:", []interface{}{user, host})
		_, err = db.ExecContext(ctx, grantStmtSQL, user, host)
		if err != nil {
			return diag.Errorf("failed setting user resource limits: %v", err)
		}
	}

	userId := fmt.Sprintf("%s@%s", user, host)
	d.SetId(userId)

	if updateStmtSql != "" {
		updateArgs := []interface{}{user, host}
		log.Println("[DEBUG] Executing statement:", updateStmtSql, "args:", updateArgs)
		_, err = db.ExecContext(ctx, updateStmtSql, updateArgs...)
		if err != nil {
			d.Set("tls_option", "")
			return diag.Errorf("failed executing SQL: %v", err)
		}
	}

	return nil
}

func getSetPasswordStatement(ctx context.Context, meta interface{}, retainPassword bool) (string, error) {
	if retainPassword {
		return "ALTER USER ?@? IDENTIFIED BY ? RETAIN CURRENT PASSWORD", nil
	}

	/* ALTER USER syntax introduced in MySQL 5.7.6 deprecates SET PASSWORD (GH-8230) */
	ver, _ := version.NewVersion("5.7.6")
	if getVersionFromMeta(ctx, meta).LessThan(ver) {
		return "SET PASSWORD FOR ?@? = PASSWORD(?)", nil
	}

	return "ALTER USER ?@? IDENTIFIED BY ?", nil
}

func UpdateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	var auth string
	if v, ok := d.GetOk("auth_plugin"); ok {
		auth = v.(string)
	}
	if len(auth) > 0 {
		if d.HasChange("tls_option") || d.HasChange("auth_plugin") || d.HasChange("auth_string_hashed") {
			stmtSQL := "ALTER USER ?@?"
			args := []interface{}{d.Get("user").(string), d.Get("host").(string)}
			authStringHashed := d.Get("auth_string_hashed").(string)
			if d.Get("auth_string_hashed").(string) != "" {
				stmtSQL += fmt.Sprintf(" IDENTIFIED WITH %s AS ?", d.Get("auth_plugin"))
				args = append(args, authStringHashed)
			}
			stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))

			log.Println("[DEBUG] Executing query:", stmtSQL, "args:", redactCreateUserArgs(args, authStringHashed))
			_, err := db.ExecContext(ctx, stmtSQL, args...)
			if err != nil {
				return diag.Errorf("failed running query: %v", err)
			}
		}
	}

	var newpw interface{}
	if d.HasChange("plaintext_password") {
		_, newpw = d.GetChange("plaintext_password")
	} else if d.HasChange("password") {
		_, newpw = d.GetChange("password")
	} else {
		newpw = nil
	}

	retainPassword := d.Get("retain_old_password").(bool)
	if retainPassword {
		err := checkRetainCurrentPasswordSupport(ctx, meta)
		if err != nil {
			return diag.Errorf("cannot use retain_current_password: %v", err)
		}
	}

	if newpw != nil {
		stmtSQL, err := getSetPasswordStatement(ctx, meta, retainPassword)
		if err != nil {
			return diag.Errorf("failed getting change password statement: %v", err)
		}

		log.Println("[DEBUG] Executing query:", stmtSQL)
		_, err = db.ExecContext(ctx, stmtSQL,
			d.Get("user").(string),
			d.Get("host").(string),
			newpw.(string))
		if err != nil {
			return diag.Errorf("failed changing password: %v", err)
		}
	}

	requiredVersion, _ := version.NewVersion("5.7.0")
	if d.HasChange("tls_option") && getVersionFromMeta(ctx, meta).GreaterThan(requiredVersion) {
		var stmtSQL string

		stmtSQL = fmt.Sprintf("ALTER USER '%s'@'%s' REQUIRE %s",
			d.Get("user").(string),
			d.Get("host").(string),
			d.Get("tls_option").(string))

		log.Println("[DEBUG] Executing query:", stmtSQL)
		_, err := db.ExecContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed setting require tls option: %v", err)
		}
	}

	if d.HasChange("max_user_connections") || d.HasChange("max_statement_time") {
		var resourceLimits []string

		if maxConn, ok := d.GetOk("max_user_connections"); ok {
			if err := checkMaxUserConnectionsSupport(ctx, meta); err != nil {
				return diag.FromErr(err)
			}
			resourceLimits = append(resourceLimits, fmt.Sprintf("MAX_USER_CONNECTIONS %d", maxConn.(int)))
		} else if d.HasChange("max_user_connections") {
			if err := checkMaxUserConnectionsSupport(ctx, meta); err != nil {
				return diag.FromErr(err)
			}
			resourceLimits = append(resourceLimits, "MAX_USER_CONNECTIONS 0")
		}

		if maxStmt, ok := d.GetOk("max_statement_time"); ok {
			if err := checkMaxStatementTimeSupport(ctx, meta); err != nil {
				return diag.FromErr(err)
			}
			resourceLimits = append(resourceLimits, fmt.Sprintf("MAX_STATEMENT_TIME %f", maxStmt.(float64)))
		} else if d.HasChange("max_statement_time") {
			isMariaDB, err := serverMariaDB(db)
			if err != nil {
				return diag.FromErr(err)
			}
			if isMariaDB {
				resourceLimits = append(resourceLimits, "MAX_STATEMENT_TIME 0")
			}
		}

		if len(resourceLimits) > 0 {
			alterUserWithVersion, _ := version.NewVersion("5.7.6")
			var stmtSQL string
			if getVersionFromMeta(ctx, meta).LessThan(alterUserWithVersion) {
				stmtSQL = "GRANT USAGE ON *.* TO ?@? WITH " + strings.Join(resourceLimits, " ")
			} else {
				stmtSQL = "ALTER USER ?@? WITH " + strings.Join(resourceLimits, " ")
			}

			args := []interface{}{d.Get("user").(string), d.Get("host").(string)}
			log.Println("[DEBUG] Executing query:", stmtSQL, "args:", args)
			_, err := db.ExecContext(ctx, stmtSQL, args...)
			if err != nil {
				return diag.Errorf("failed setting user resource limits: %v", err)
			}
		}
	}

	if tiDBUserOptionsChanged(d) {
		clauses := buildTiDBUserOptionClauses(d, true)
		if len(clauses) > 0 {
			stmtSQL := fmt.Sprintf("ALTER USER '%s'@'%s' %s",
				d.Get("user").(string),
				d.Get("host").(string),
				strings.Join(clauses, " "))

			log.Println("[DEBUG] Executing query:", stmtSQL)
			_, err := db.ExecContext(ctx, stmtSQL)
			if err != nil {
				return diag.Errorf("failed setting TiDB user options: %v", err)
			}
		}
	}

	return nil
}

func parseWithClauseSetting(d *schema.ResourceData, withClause, fieldName, settingName string, parseAsFloat bool) {
	if _, ok := d.GetOk(fieldName); !ok {
		return
	}

	pattern := fmt.Sprintf(`%s\s+([\d.]+)`, settingName)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(withClause)
	if len(match) <= 1 {
		return
	}

	if parseAsFloat {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil {
			d.Set(fieldName, value)
		}
		return
	}

	if value, err := strconv.Atoi(match[1]); err == nil {
		d.Set(fieldName, value)
	}
}

func ReadUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	requiredVersion, _ := version.NewVersion("5.7.0")
	if getVersionFromMeta(ctx, meta).GreaterThan(requiredVersion) {
		stmt := "SHOW CREATE USER ?@?"

		var createUserStmt string
		err := db.QueryRowContext(ctx, stmt, d.Get("user").(string), d.Get("host").(string)).Scan(&createUserStmt)
		if err != nil {
			errorNumber := mysqlErrorNumber(err)
			if errorNumber == unknownUserErrCode || errorNumber == userNotFoundErrCode {
				d.SetId("")
				return nil
			}
			return diag.Errorf("failed getting user: %v", err)
		}

		// Examples of create user:
		// CREATE USER 'some_app'@'%' IDENTIFIED WITH 'mysql_native_password' AS '*0something' REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK
		// CREATE USER `jdoe-tf-test-47`@`example.com` IDENTIFIED WITH 'caching_sha2_password' REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT PASSWORD REQUIRE CURRENT DEFAULT
		// CREATE USER `jdoe`@`example.com` IDENTIFIED WITH 'caching_sha2_password' AS '$A$005$i`xay#fG/\' TrbkNA82' REQUIRE NONE PASSWORD
		re := regexp.MustCompile("^CREATE USER ['`]([^'`]*)['`]@['`]([^'`]*)['`] IDENTIFIED WITH ['`]([^'`]*)['`] (?:AS '((?:.*?[^\\\\])?)' )?REQUIRE ([^ ]*)")
		if m := re.FindStringSubmatch(createUserStmt); len(m) == 6 {
			d.Set("user", m[1])
			d.Set("host", m[2])
			d.Set("auth_plugin", m[3])
			d.Set("tls_option", m[5])

			if m[3] == "aad_auth" {
				// AADGroup:98e61c8d-e104-4f8c-b1a6-7ae873617fe6:upn:Doe_Family_Group
				// AADUser:98e61c8d-e104-4f8c-b1a6-7ae873617fe6:upn:little.johny@does.onmicrosoft.com
				// AADSP:98e61c8d-e104-4f8c-b1a6-7ae873617fe6:upn:mysqlUserName - for MySQL Flexible Server
				// AADApp:98e61c8d-e104-4f8c-b1a6-7ae873617fe6:upn:mysqlUserName - for MySQL Single Server
				parts := strings.Split(m[4], ":")
				if parts[0] == "AADSP" || parts[0] == "AADApp" {
					// service principals are referenced by UUID only
					d.Set("aad_identity", []map[string]interface{}{
						{
							"type":     "service_principal",
							"identity": parts[1],
						},
					})
				} else if len(parts) >= 4 {
					// users and groups should be referenced by UPN / group name
					if parts[0] == "AADUser" {
						d.Set("aad_identity", []map[string]interface{}{
							{
								"type":     "user",
								"identity": strings.Join(parts[3:], ":"),
							},
						})
					} else {
						d.Set("aad_identity", []map[string]interface{}{
							{
								"type":     "group",
								"identity": strings.Join(parts[3:], ":"),
							},
						})
					}
				} else {
					return diag.Errorf("AAD identity couldn't be parsed - it is %s", m[4])
				}
			} else {
				d.Set("auth_string_hashed", m[4])
			}

			withRe := regexp.MustCompile(`WITH\s+(.*)$`)
			if withMatch := withRe.FindStringSubmatch(createUserStmt); len(withMatch) > 1 {
				withClause := withMatch[1]
				parseWithClauseSetting(d, withClause, "max_user_connections", "MAX_USER_CONNECTIONS", false)
				parseWithClauseSetting(d, withClause, "max_statement_time", "MAX_STATEMENT_TIME", true)
			}

			setTiDBUserOptionsFromCreateStatement(createUserStmt, d)
			if err := readTiDBMaxUserConnections(ctx, db, d); err != nil {
				return diag.Errorf("failed reading TiDB max_user_connections: %v", err)
			}
			return nil
		}

		// Try 2 - just whether the user is there.
		re2 := regexp.MustCompile("^CREATE USER")
		if m := re2.FindStringSubmatch(createUserStmt); m != nil {
			// Ok, we have at least something - it's probably in MariaDB.
			withRe := regexp.MustCompile(`WITH\s+(.*)$`)
			if withMatch := withRe.FindStringSubmatch(createUserStmt); len(withMatch) > 1 {
				withClause := withMatch[1]
				parseWithClauseSetting(d, withClause, "max_user_connections", "MAX_USER_CONNECTIONS", false)
				parseWithClauseSetting(d, withClause, "max_statement_time", "MAX_STATEMENT_TIME", true)
			}

			return nil
		}
		return diag.Errorf("Create user couldn't be parsed - it is %s", createUserStmt)
	} else {
		// Worse user detection, only for compat with MySQL 5.6
		stmtSQL := fmt.Sprintf("SELECT USER FROM mysql.user WHERE USER='%s'",
			d.Get("user").(string))

		log.Println("[DEBUG] Executing statement:", stmtSQL)

		rows, err := db.QueryContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed getting user from DB: %v", err)
		}
		defer rows.Close()

		if !rows.Next() && rows.Err() == nil {
			d.SetId("")
			return nil
		}
		if rows.Err() != nil {
			return diag.Errorf("failed getting rows: %v", rows.Err())
		}
	}
	return nil
}

func DeleteUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := fmt.Sprintf("DROP USER ?@?")

	log.Println("[DEBUG] Executing statement:", stmtSQL)

	_, err = db.ExecContext(ctx, stmtSQL,
		d.Get("user").(string),
		d.Get("host").(string))

	if err == nil {
		d.SetId("")
	}
	return diag.FromErr(err)
}

func ImportUser(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	userHost := strings.SplitN(d.Id(), "@", 2)

	if len(userHost) != 2 {
		return nil, fmt.Errorf("wrong ID format %s (expected USER@HOST)", d.Id())
	}

	user := userHost[0]
	host := userHost[1]
	d.Set("user", user)
	d.Set("host", host)
	err := ReadUser(ctx, d, meta)
	var ferror error
	if err.HasError() {
		ferror = fmt.Errorf("failed reading user: %v", err)
	}

	return []*schema.ResourceData{d}, ferror
}

func NewEmptyStringSuppressFunc(k, old, new string, d *schema.ResourceData) bool {
	if new == "" {
		return true
	}

	return false
}

func appendTiDBUserOptionClauses(stmtSQL string, clauses []string) string {
	if len(clauses) == 0 {
		return stmtSQL
	}

	return stmtSQL + " " + strings.Join(clauses, " ")
}

func tiDBUserOptionsChanged(d *schema.ResourceData) bool {
	for _, key := range []string{
		"resource_group",
		"account_locked",
		"comment",
		"attribute_json",
		"password_expire",
		"password_history",
		"password_reuse_interval",
		"failed_login_attempts",
		"password_lock_time",
	} {
		if d.HasChange(key) {
			return true
		}
	}

	return false
}

func buildTiDBUserOptionClauses(d *schema.ResourceData, includeClears bool) []string {
	clauses := []string{}

	if value, ok := d.GetOk("password_expire"); ok {
		clauses = append(clauses, "PASSWORD EXPIRE "+strings.ToUpper(value.(string)))
	}

	if value, ok := d.GetOk("password_history"); ok {
		clauses = append(clauses, "PASSWORD HISTORY "+strings.ToUpper(value.(string)))
	}

	if value, ok := d.GetOk("password_reuse_interval"); ok {
		clauses = append(clauses, "PASSWORD REUSE INTERVAL "+strings.ToUpper(value.(string)))
	}

	if value, ok := d.GetOk("failed_login_attempts"); ok {
		clauses = append(clauses, fmt.Sprintf("FAILED_LOGIN_ATTEMPTS %d", value.(int)))
	}

	if value, ok := d.GetOk("password_lock_time"); ok {
		clauses = append(clauses, "PASSWORD_LOCK_TIME "+strings.ToUpper(value.(string)))
	}

	if value, ok := d.GetOkExists("account_locked"); ok {
		if value.(bool) {
			clauses = append(clauses, "ACCOUNT LOCK")
		} else {
			clauses = append(clauses, "ACCOUNT UNLOCK")
		}
	}

	if value, ok := d.GetOk("comment"); ok {
		clauses = append(clauses, "COMMENT "+quoteSQLString(value.(string)))
	} else if includeClears && d.HasChange("comment") {
		clauses = append(clauses, "COMMENT ''")
	}

	if value, ok := d.GetOk("attribute_json"); ok {
		clauses = append(clauses, "ATTRIBUTE "+quoteSQLString(value.(string)))
	} else if includeClears && d.HasChange("attribute_json") {
		clauses = append(clauses, "ATTRIBUTE '{}'")
	}

	if value, ok := d.GetOk("resource_group"); ok {
		clauses = append(clauses, "RESOURCE GROUP "+quoteIdentifier(value.(string)))
	} else if includeClears && d.HasChange("resource_group") {
		clauses = append(clauses, "RESOURCE GROUP `default`")
	}

	return clauses
}

var sqlStringQuoteReplacer = strings.NewReplacer(`\`, `\\`, `'`, `''`)

func quoteSQLString(value string) string {
	return "'" + sqlStringQuoteReplacer.Replace(value) + "'"
}

func setTiDBUserOptionsFromCreateStatement(createUserStmt string, d *schema.ResourceData) {
	if match := regexp.MustCompile(`RESOURCE GROUP [` + "`" + `']?([^` + "`" + `'\s]+)[` + "`" + `']?`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("resource_group", match[1])
	}

	if strings.Contains(createUserStmt, " ACCOUNT LOCK") {
		d.Set("account_locked", true)
	} else if strings.Contains(createUserStmt, " ACCOUNT UNLOCK") {
		d.Set("account_locked", false)
	}

	if match := regexp.MustCompile(`COMMENT '((?:''|[^'])*)'`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("comment", unquoteSQLString(match[1]))
	}

	if match := regexp.MustCompile(`ATTRIBUTE '((?:''|[^'])*)'`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("attribute_json", unquoteSQLString(match[1]))
	}

	if match := regexp.MustCompile(`PASSWORD EXPIRE (DEFAULT|NEVER|INTERVAL [0-9]+ DAY)`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("password_expire", strings.ToLower(match[1]))
	}

	if match := regexp.MustCompile(`PASSWORD HISTORY (DEFAULT|[0-9]+)`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("password_history", strings.ToLower(match[1]))
	}

	if match := regexp.MustCompile(`PASSWORD REUSE INTERVAL (DEFAULT|[0-9]+ DAY)`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("password_reuse_interval", strings.ToLower(match[1]))
	}

	if match := regexp.MustCompile(`FAILED_LOGIN_ATTEMPTS ([0-9]+)`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		var value int
		if _, err := fmt.Sscanf(match[1], "%d", &value); err == nil {
			d.Set("failed_login_attempts", value)
		}
	}

	if match := regexp.MustCompile(`PASSWORD_LOCK_TIME (UNBOUNDED|[0-9]+)`).FindStringSubmatch(createUserStmt); len(match) == 2 {
		d.Set("password_lock_time", strings.ToLower(match[1]))
	}
}

func unquoteSQLString(value string) string {
	return strings.ReplaceAll(value, "''", "'")
}
