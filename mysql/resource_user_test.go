package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Uses shared container set up in TestMain
// Skips MariaDB (same as original test)
func TestAccUser_basic(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSkipMariaDB(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "%"),
					resource.TestCheckResourceAttr("mysql_user.test", "plaintext_password", hashSum("password")),
					resource.TestCheckResourceAttr("mysql_user.test", "tls_option", "NONE"),
				),
			},
			{
				Config: testAccUserConfig_ssl,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "plaintext_password", hashSum("password")),
					resource.TestCheckResourceAttr("mysql_user.test", "tls_option", "SSL"),
				),
			},
			{
				Config: testAccUserConfig_newPass,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "%"),
					resource.TestCheckResourceAttr("mysql_user.test", "plaintext_password", hashSum("password2")),
					resource.TestCheckResourceAttr("mysql_user.test", "tls_option", "NONE"),
				),
			},
		},
	})
}

// Requires MySQL (not TiDB/MariaDB/RDS) with mysql_no_login plugin
// Uses shared container set up in TestMain
// Note: mysql_no_login plugin may not be available in all MySQL distributions
func TestAccUser_auth(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckSkipTiDB(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipRds(t)
			// Check if mysql_no_login plugin is available
			ctx := context.Background()
			db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
			if err != nil {
				t.Fatalf("Cannot connect to DB: %v", err)
			}
			// Don't close - connection is cached and shared

			// Check if plugin exists
			var pluginName string
			err = db.QueryRowContext(ctx, "SELECT PLUGIN_NAME FROM INFORMATION_SCHEMA.PLUGINS WHERE PLUGIN_NAME = 'mysql_no_login'").Scan(&pluginName)
			if err != nil {
				t.Skip("mysql_no_login plugin is not available in this MySQL distribution")
			}
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_auth_iam_plugin,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_plugin", "mysql_no_login"),
				),
			},
			{
				Config: testAccUserConfig_auth_native,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_plugin", "mysql_native_password"),
				),
			},
			{
				Config: testAccUserConfig_auth_iam_plugin,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_plugin", "mysql_no_login"),
				),
			},
		},
	})
}

// Requires MySQL (not TiDB/MariaDB/RDS)
// Uses shared container set up in TestMain
func TestAccUser_authConnect(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipTiDB(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipRds(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password"),
				),
			},
			{
				Config: testAccUserConfig_newPass,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "random"),
				),
				ExpectError: regexp.MustCompile(`.*Access denied for user 'jdoe'.*`),
			},
			{
				Config: testAccUserConfig_newPass,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password"),
				),
				ExpectError: regexp.MustCompile(`.*Access denied for user 'jdoe'.*`),
			},
			{
				Config: testAccUserConfig_newPass,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password2"),
				),
			},
		},
	})
}

// Requires MySQL 8.0.14+ (not MariaDB/RDS)
// Uses shared container set up in TestMain
func TestAccUser_authConnectRetainOldPassword(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipRds(t)
			testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.14")
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_basic_retain_old_password,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password"),
				),
			},
			{
				Config: testAccUserConfig_newPass_retain_old_password,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password"),
					testAccUserAuthValid("jdoe", "password2"),
				),
			},
			{
				Config: testAccUserConfig_newNewPass_retain_old_password,
				Check: resource.ComposeTestCheckFunc(
					testAccUserAuthValid("jdoe", "password"),
				),
				ExpectError: regexp.MustCompile(`.*Access denied for user 'jdoe'.*`),
			},
		},
	})
}

// Uses shared container set up in TestMain
func TestAccUser_deprecated(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_deprecated,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "password", "password"),
				),
			},
			{
				Config: testAccUserConfig_deprecated_newPass,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "jdoe"),
					resource.TestCheckResourceAttr("mysql_user.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_user.test", "password", "password2"),
				),
			},
		},
	})
}

func testAccUserCheckDestroy(s *terraform.State) error {
	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "mysql_user" {
			continue
		}
		stmtSQL := fmt.Sprintf("SELECT user from mysql.user where CONCAT(user, '@', host) = '%s'", rs.Primary.ID)
		log.Println("[DEBUG] Executing statement:", stmtSQL)
		rows, err := db.Query(stmtSQL)
		if err != nil {
			return fmt.Errorf("error issuing query: %s", err)
		}
		haveNext := rows.Next()
		rows.Close()
		if haveNext {
			return fmt.Errorf("user still exists after destroy")
		}
	}
	return nil
}

func testAccUserExists(rn string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource not found: %s", rn)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("user id not set")
		}
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}
		stmtSQL := fmt.Sprintf("SELECT count(*) from mysql.user where CONCAT(user, '@', host) = '%s'", rs.Primary.ID)
		log.Println("[DEBUG] Executing statement:", stmtSQL)
		var count int
		err = db.QueryRow(stmtSQL).Scan(&count)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("expected 1 row reading user but got no rows")
			}
			return fmt.Errorf("error reading user: %s", err)
		}
		return nil
	}
}

const testAccUserConfig_basic = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "%"
    plaintext_password = "password"
}
`

const testAccUserConfig_ssl = `
resource "mysql_user" "test" {
	user = "jdoe"
	host = "example.com"
	plaintext_password = "password"
	tls_option = "SSL"
}
`

const testAccUserConfig_newPass = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "%"
    plaintext_password = "password2"
}
`

const testAccUserConfig_auth_iam_plugin = `
resource "mysql_user" "test" {
	user = "jdoe"
	host = "example.com"
	auth_plugin = "mysql_no_login"
}
`

const testAccUserConfig_auth_native = `
resource "mysql_user" "test" {
	user = "jdoe"
	host = "example.com"
	auth_plugin = "mysql_native_password"
	plaintext_password = "password"
}
`

const testAccUserConfig_deprecated = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "example.com"
    password = "password"
}
`

const testAccUserConfig_deprecated_newPass = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "example.com"
    password = "password2"
}
`

func testAccUserAuthExists(rn string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource not found: %s", rn)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("user id not set")
		}
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}
		stmtSQL := fmt.Sprintf("SELECT count(*) from mysql.user where CONCAT(user, '@', host) = '%s'", rs.Primary.ID)
		log.Println("[DEBUG] Executing statement:", stmtSQL)
		var count int
		err = db.QueryRow(stmtSQL).Scan(&count)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("expected 1 row reading user but got no rows")
			}
			return fmt.Errorf("error reading user: %s", err)
		}
		return nil
	}
}

const testAccUserConfig_basic_retain_old_password = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "%"
    plaintext_password = "password"
    retain_old_password = true
}
`

const testAccUserConfig_newPass_retain_old_password = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "%"
    plaintext_password = "password2"
    retain_old_password = true
}
`

const testAccUserConfig_newNewPass_retain_old_password = `
resource "mysql_user" "test" {
    user = "jdoe"
    host = "%"
    plaintext_password = "password3"
    retain_old_password = true
}
`

func testAccUserAuthValid(user string, password string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		userConf := testAccProvider.Meta().(*MySQLConfiguration)
		userConf.Config.User = user
		userConf.Config.Passwd = password
		ctx := context.Background()
		connection, err := createNewConnection(ctx, userConf)
		if err != nil {
			return fmt.Errorf("could not create new connection: %v", err)
		}
		connection.Db.Close()
		return nil
	}
}
