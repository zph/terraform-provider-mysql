package mysql

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

			"auth_string_hashed": {
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				DiffSuppressFunc: NewEmptyStringSuppressFunc,
				RequiredWith:     []string{"auth_plugin"},
				ConflictsWith:    []string{"plaintext_password", "password"},
			},

			"tls_option": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "NONE",
			},
		},
	}
}

func CreateUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	var authStm string
	var auth string
	if v, ok := d.GetOk("auth_plugin"); ok {
		auth = v.(string)
	}

	if len(auth) > 0 {
		if auth == "AWSAuthenticationPlugin" {
			authStm = " IDENTIFIED WITH AWSAuthenticationPlugin as 'RDS'"
		} else {
			// mysql_no_login, auth_pam, ...
			authStm = " IDENTIFIED WITH " + auth
		}
	}
	if v, ok := d.GetOk("auth_string_hashed"); ok {
		hashed := v.(string)
		if hashed != "" {
			authStm = fmt.Sprintf("%s AS '%s'", authStm, hashed)
		}
	}

	stmtSQL := fmt.Sprintf("CREATE USER '%s'@'%s'",
		d.Get("user").(string),
		d.Get("host").(string))

	var password string
	if v, ok := d.GetOk("plaintext_password"); ok {
		password = v.(string)
	} else {
		password = d.Get("password").(string)
	}

	if auth == "AWSAuthenticationPlugin" && d.Get("host").(string) == "localhost" {
		return diag.Errorf("cannot use IAM auth against localhost")
	}

	if authStm != "" {
		stmtSQL = stmtSQL + authStm
	} else {
		stmtSQL = stmtSQL + fmt.Sprintf(" IDENTIFIED BY '%s'", password)
	}

	requiredVersion, _ := version.NewVersion("5.7.0")

	if getVersionFromMeta(ctx, meta).GreaterThan(requiredVersion) && d.Get("tls_option").(string) != "" {
		stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
	}

	log.Println("Executing statement:", stmtSQL)
	_, err = db.ExecContext(ctx, stmtSQL)
	if err != nil {
		return diag.Errorf("failed executing SQL: %v", err)
	}

	user := fmt.Sprintf("%s@%s", d.Get("user").(string), d.Get("host").(string))
	d.SetId(user)

	return nil
}

func getSetPasswordStatement(ctx context.Context, meta interface{}) (string, error) {
	/* ALTER USER syntax introduced in MySQL 5.7.6 deprecates SET PASSWORD (GH-8230) */
	ver, _ := version.NewVersion("5.7.6")
	if getVersionFromMeta(ctx, meta).LessThan(ver) {
		return "SET PASSWORD FOR ?@? = PASSWORD(?)", nil
	} else {
		return "ALTER USER ?@? IDENTIFIED BY ?", nil
	}
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
			var stmtSQL string

			authString := ""
			if d.Get("auth_string_hashed").(string) != "" {
				authString = fmt.Sprintf("IDENTIFIED WITH %s AS '%s'", d.Get("auth_plugin"), d.Get("auth_string_hashed"))
			}
			stmtSQL = fmt.Sprintf("ALTER USER '%s'@'%s' %s  REQUIRE %s",
				d.Get("user").(string),
				d.Get("host").(string),
				authString,
				d.Get("tls_option").(string))

			log.Println("Executing query:", stmtSQL)
			_, err := db.ExecContext(ctx, stmtSQL)
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

	if newpw != nil {
		stmtSQL, err := getSetPasswordStatement(ctx, meta)
		if err != nil {
			return diag.Errorf("failed getting change password statement: %v", err)
		}

		log.Println("Executing query:", stmtSQL)
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

		log.Println("Executing query:", stmtSQL)
		_, err := db.ExecContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed setting require tls option: %v", err)
		}
	}

	return nil
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
			if mysqlErrorNumber(err) == unknownUserErrCode {
				d.SetId("")
				return nil
			}
			return diag.Errorf("failed getting version: %v", err)
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
			d.Set("auth_string_hashed", m[4])
			d.Set("tls_option", m[5])
			return nil
		}

		// Try 2 - just whether the user is there.
		re2 := regexp.MustCompile("^CREATE USER")
		if m := re2.FindStringSubmatch(createUserStmt); m != nil {
			// Ok, we have at least something - it's probably in MariaDB.
			return nil
		}
		return diag.Errorf("Create user couldn't be parsed - it is %s", createUserStmt)
	} else {
		// Worse user detection, only for compat with MySQL 5.6
		stmtSQL := fmt.Sprintf("SELECT USER FROM mysql.user WHERE USER='%s'",
			d.Get("user").(string))

		log.Println("Executing statement:", stmtSQL)

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

	log.Println("Executing statement:", stmtSQL)

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
