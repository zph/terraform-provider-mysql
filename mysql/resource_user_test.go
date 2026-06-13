//go:build testcontainers
// +build testcontainers

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
			// Skip on MySQL 8.0+ and Percona 8.0+ due to auth_plugin conflict with plaintext_password
			testAccPreCheckSkipNotMySQLVersionMax(t, "7.99.99")
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
		PreCheck: func() {
			testAccPreCheck(t)
			// Skip on MySQL 8.0+ and Percona 8.0+ due to stricter user creation requirements
			testAccPreCheckSkipNotMySQLVersionMax(t, "7.99.99")
		},
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

func TestAccUser_resourceLimits(t *testing.T) {
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipTiDBVersionLessThan(t, tiDBMaxUserConnectionsMinVersion)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_resourceLimits,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "limited_user"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_user_connections", "10"),
					testAccUserResourceLimitsMaxConn("limited_user", "%", 10),
				),
			},
			{
				Config: testAccUserConfig_resourceLimitsUpdated,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_user_connections", "20"),
					testAccUserResourceLimitsMaxConn("limited_user", "%", 20),
				),
			},
			{
				Config: testAccUserConfig_resourceLimitsRemoved,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					testAccUserResourceLimitsMaxConn("limited_user", "%", 0),
				),
			},
		},
	})
}

func TestAccUser_resourceLimitsMariaDB(t *testing.T) {
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckRequireMariaDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_resourceLimitsMariaDB,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "user", "limited_user_mariadb"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_user_connections", "15"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_statement_time", "30.5"),
					testAccUserResourceLimitsMariaDB("limited_user_mariadb", "%", 15, 30.5),
				),
			},
			{
				Config: testAccUserConfig_resourceLimitsMariaDBUpdated,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_user_connections", "25"),
					resource.TestCheckResourceAttr("mysql_user.test", "max_statement_time", "45.5"),
					testAccUserResourceLimitsMariaDB("limited_user_mariadb", "%", 25, 45.5),
				),
			},
			{
				Config: testAccUserConfig_resourceLimitsMariaDBRemoved,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					testAccUserResourceLimitsMariaDB("limited_user_mariadb", "%", 0, 0),
				),
			},
		},
	})
}

func TestAccUser_resourceLimitsErrorOnUnsupportedTiDB(t *testing.T) {
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckSkipTiDBVersionGreaterThanOrEqual(t, tiDBMaxUserConnectionsMinVersion)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccUserConfig_resourceLimitsUnsupportedTiDB,
				ExpectError: regexp.MustCompile("MAX_USER_CONNECTIONS is only supported on TiDB 8.5.5 or newer"),
			},
		},
	})
}

func TestAccUser_resourceLimitsErrorOnMySQL(t *testing.T) {
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipMariaDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccUserConfig_resourceLimitsErrorMySQL,
				ExpectError: regexp.MustCompile("MAX_STATEMENT_TIME is only supported on MariaDB"),
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

func testAccUserResourceLimitsMaxConn(user, host string, expectedMaxConn int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		isTiDB, _, _, err := serverTiDB(db)
		if err != nil {
			return err
		}
		if isTiDB {
			var createUserStmt string
			err = db.QueryRowContext(ctx, "SHOW CREATE USER ?@?", user, host).Scan(&createUserStmt)
			if err != nil {
				return fmt.Errorf("error reading TiDB user resource limits: %s", err)
			}

			maxUserConn, found, err := parseMaxUserConnectionsFromCreateUserStatement(createUserStmt)
			if err != nil {
				return fmt.Errorf("error parsing TiDB user resource limits: %s", err)
			}
			if !found {
				maxUserConn = 0
			}
			if maxUserConn != expectedMaxConn {
				return fmt.Errorf("expected max_user_connections %d, got %d", expectedMaxConn, maxUserConn)
			}

			return nil
		}

		var maxUserConn int
		query := fmt.Sprintf("SELECT max_user_connections FROM mysql.user WHERE user='%s' AND host='%s'", user, host)
		err = db.QueryRow(query).Scan(&maxUserConn)
		if err != nil {
			return fmt.Errorf("error reading user resource limits: %s", err)
		}
		if maxUserConn != expectedMaxConn {
			return fmt.Errorf("expected max_user_connections %d, got %d", expectedMaxConn, maxUserConn)
		}

		return nil
	}
}

func testAccUserResourceLimitsMariaDB(user, host string, expectedMaxConn int, expectedMaxStmt float64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		var maxUserConn int
		var maxStmtTime float64
		query := fmt.Sprintf("SELECT max_user_connections, max_statement_time FROM mysql.user WHERE user='%s' AND host='%s'", user, host)
		err = db.QueryRow(query).Scan(&maxUserConn, &maxStmtTime)
		if err != nil {
			return fmt.Errorf("error reading user resource limits: %s", err)
		}
		if maxUserConn != expectedMaxConn {
			return fmt.Errorf("expected max_user_connections %d, got %d", expectedMaxConn, maxUserConn)
		}
		if maxStmtTime != expectedMaxStmt {
			return fmt.Errorf("expected max_statement_time %f, got %f", expectedMaxStmt, maxStmtTime)
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

const testAccUserConfig_resourceLimits = `
resource "mysql_user" "test" {
    user                 = "limited_user"
    host                 = "%"
    plaintext_password   = "password"
    max_user_connections = 10
}
`

const testAccUserConfig_resourceLimitsUpdated = `
resource "mysql_user" "test" {
    user                 = "limited_user"
    host                 = "%"
    plaintext_password   = "password"
    max_user_connections = 20
}
`

const testAccUserConfig_resourceLimitsRemoved = `
resource "mysql_user" "test" {
    user               = "limited_user"
    host               = "%"
    plaintext_password = "password"
}
`

const testAccUserConfig_resourceLimitsMariaDB = `
resource "mysql_user" "test" {
    user                 = "limited_user_mariadb"
    host                 = "%"
    plaintext_password   = "password"
    max_user_connections = 15
    max_statement_time   = 30.5
}
`

const testAccUserConfig_resourceLimitsMariaDBUpdated = `
resource "mysql_user" "test" {
    user                 = "limited_user_mariadb"
    host                 = "%"
    plaintext_password   = "password"
    max_user_connections = 25
    max_statement_time   = 45.5
}
`

const testAccUserConfig_resourceLimitsMariaDBRemoved = `
resource "mysql_user" "test" {
    user               = "limited_user_mariadb"
    host               = "%"
    plaintext_password = "password"
}
`

const testAccUserConfig_resourceLimitsUnsupportedTiDB = `
resource "mysql_user" "test" {
    user                 = "unsupported_tidb_user"
    host                 = "%"
    plaintext_password   = "password"
    max_user_connections = 10
}
`

const testAccUserConfig_resourceLimitsErrorMySQL = `
resource "mysql_user" "test" {
    user               = "error_user"
    host               = "%"
    plaintext_password = "password"
    max_statement_time = 30.0
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
