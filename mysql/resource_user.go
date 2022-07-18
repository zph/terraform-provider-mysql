package mysql

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"errors"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceUser() *schema.Resource {
	return &schema.Resource{
		Create: CreateUser,
		Update: UpdateUser,
		Read:   ReadUser,
		Delete: DeleteUser,
		Importer: &schema.ResourceImporter{
			State: ImportUser,
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

func CreateUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

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
		return errors.New("cannot use IAM auth against localhost")
	}

	if authStm != "" {
		stmtSQL = stmtSQL + authStm
	} else {
		stmtSQL = stmtSQL + fmt.Sprintf(" IDENTIFIED BY '%s'", password)
	}

	requiredVersion, _ := version.NewVersion("5.7.0")

	if meta.(*MySQLConfiguration).Version.GreaterThan(requiredVersion) && d.Get("tls_option").(string) != "" {
		stmtSQL += fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string))
	}

	log.Println("Executing statement:", stmtSQL)
	_, err := db.Exec(stmtSQL)
	if err != nil {
		return err
	}

	user := fmt.Sprintf("%s@%s", d.Get("user").(string), d.Get("host").(string))
	d.SetId(user)

	return nil
}

func getSetPasswordStatement(meta interface{}) (string, error) {
	/* ALTER USER syntax introduced in MySQL 5.7.6 deprecates SET PASSWORD (GH-8230) */
	ver, _ := version.NewVersion("5.7.6")
	if meta.(*MySQLConfiguration).Version.LessThan(ver) {
		return "SET PASSWORD FOR ?@? = PASSWORD(?)", nil
	} else {
		return "ALTER USER ?@? IDENTIFIED BY ?", nil
	}
}

func UpdateUser(d *schema.ResourceData, meta interface{}) error {
	mysqlConf := meta.(*MySQLConfiguration)
	db := mysqlConf.Db

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
			_, err := db.Exec(stmtSQL)
			if err != nil {
				return err
			}
		}

		// nothing to change, return
		return nil
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
		stmtSQL, err := getSetPasswordStatement(meta)
		if err != nil {
			return err
		}

		log.Println("Executing query:", stmtSQL)
		_, err = db.Exec(stmtSQL,
			d.Get("user").(string),
			d.Get("host").(string),
			newpw.(string))
		if err != nil {
			return err
		}
	}

	requiredVersion, _ := version.NewVersion("5.7.0")
	if d.HasChange("tls_option") && mysqlConf.Version.GreaterThan(requiredVersion) {
		var stmtSQL string

		stmtSQL = fmt.Sprintf("ALTER USER '%s'@'%s'  REQUIRE %s",
			d.Get("user").(string),
			d.Get("host").(string),
			fmt.Sprintf(" REQUIRE %s", d.Get("tls_option").(string)))

		log.Println("Executing query:", stmtSQL)
		_, err := db.Exec(stmtSQL)
		if err != nil {
			return err
		}
	}

	return nil
}

func ReadUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	currentVersion := meta.(*MySQLConfiguration).Version
	requiredVersion, _ := version.NewVersion("5.7.0")
	if currentVersion.GreaterThan(requiredVersion) {
		stmt := "SHOW CREATE USER ?@?"

		var createUserStmt string
		err := db.QueryRow(stmt, d.Get("user").(string), d.Get("host").(string)).Scan(&createUserStmt)
		if err != nil {
			if mysqlErrorNumber(err) == unknownUserErrCode {
				d.SetId("")
				return nil
			}
			return err
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
		} else {
			return fmt.Errorf("Create user couldn't be parsed - it is %s", createUserStmt)
		}
		return nil
	} else {
		// Worse user detection, only for compat with MySQL 5.6
		stmtSQL := fmt.Sprintf("SELECT USER FROM mysql.user WHERE USER='%s'",
			d.Get("user").(string))

		log.Println("Executing statement:", stmtSQL)

		rows, err := db.Query(stmtSQL)
		if err != nil {
			return err
		}
		defer rows.Close()

		if !rows.Next() && rows.Err() == nil {
			d.SetId("")
			return rows.Err()
		}
	}
	return nil
}

func DeleteUser(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	stmtSQL := fmt.Sprintf("DROP USER ?@?")

	log.Println("Executing statement:", stmtSQL)

	_, err := db.Exec(stmtSQL,
		d.Get("user").(string),
		d.Get("host").(string))

	if err == nil {
		d.SetId("")
	}
	return err
}

func ImportUser(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	userHost := strings.SplitN(d.Id(), "@", 2)

	if len(userHost) != 2 {
		return nil, fmt.Errorf("wrong ID format %s (expected USER@HOST)", d.Id())
	}

	user := userHost[0]
	host := userHost[1]
	d.Set("user", user)
	d.Set("host", host)
	err := ReadUser(d, meta)

	return []*schema.ResourceData{d}, err
}

func NewEmptyStringSuppressFunc(k, old, new string, d *schema.ResourceData) bool {
	if new == "" {
		return true
	}

	return false
}
