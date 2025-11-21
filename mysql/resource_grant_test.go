//go:build testcontainers
// +build testcontainers

package mysql

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Uses shared container set up in TestMain
// Skips RDS (same as original test)
func TestAccGrant(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	userName := fmt.Sprintf("jdoe-%s", dbName)
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSkipRds(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", userName),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "*"),
				),
			},
			{
				Config: testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", userName),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
				),
			},
			{
				Config:            testAccGrantConfigBasic(dbName),
				ResourceName:      "mysql_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%v@%v@%v@%v", userName, "example.com", dbName, "*"),
			},
		},
	})
}

// Skips RDS (same as original test)
func TestAccRevokePrivRefresh(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSkipRds(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "UPDATE", true, false),
				),
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					revokeUserPrivs(dbName, "UPDATE"),
				),
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "UPDATE", false, false),
				),
			},
			{
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Config:             testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "UPDATE", false, false),
				),
			},
			{
				Config: testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "UPDATE", true, false),
				),
			},
		},
	})
}

func TestAccBroken(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigBasic(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "*"),
				),
			},
			{
				Config:      testAccGrantConfigBroken(dbName),
				ExpectError: regexp.MustCompile("already has"),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "*"),
				),
			},
		},
	})
}

// Skips TiDB (same as original test)
func TestAccDifferentHosts(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigExtraHost(dbName, false),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test_all", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "host", "%"),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "table", "*"),
				),
			},
			{
				Config: testAccGrantConfigExtraHost(dbName, true),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "10.1.2.3"),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "*"),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "host", "%"),
					resource.TestCheckResourceAttr("mysql_grant.test_all", "table", "*"),
				),
			},
		},
	})
}

// Skips TiDB, RDS (same as original test)
func TestAccGrantComplex(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipTiDB(t)
			testAccPreCheckSkipRds(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				// Create table first
				Config: testAccGrantConfigNoGrant(dbName),
				Check: resource.ComposeTestCheckFunc(
					prepareTable(dbName, "tbl"),
				),
			},
			{
				Config: testAccGrantConfigWithPrivs(dbName, `"SELECT (c1, c2)"`, false),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT (c1,c2)", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "tbl"),
				),
			},
			{
				Config: testAccGrantConfigWithPrivs(dbName, `"DROP", "SELECT (c1)", "INSERT(c3, c4)", "REFERENCES(c5)"`, false),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "INSERT (c3,c4)", true, false),
					testAccPrivilege("mysql_grant.test", "SELECT (c1)", true, false),
					testAccPrivilege("mysql_grant.test", "SELECT (c1,c2)", false, false),
					testAccPrivilege("mysql_grant.test", "REFERENCES (c5)", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "tbl"),
				),
			},
			{
				Config: testAccGrantConfigWithPrivs(dbName, `"ALL PRIVILEGES"`, false),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "ALL", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "tbl"),
				),
			},
			{
				Config: testAccGrantConfigWithPrivs(dbName, `"SELECT (c1, c2)","UPDATE(c1, c2)"`, true),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "SELECT (c1,c2)", true, true),
					testAccPrivilege("mysql_grant.test", "UPDATE (c1,c2)", true, true),
					testAccPrivilege("mysql_grant.test", "ALL", false, true),
					testAccPrivilege("mysql_grant.test", "DROP", false, true),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", dbName),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "tbl"),
				),
			},
			{
				Config: testAccGrantConfigNoGrant(dbName),
			},
		},
	})
}

// Skips RDS, MariaDB, MySQL < 8.0, TiDB (same as original test)
func TestAccGrantComplexMySQL8(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipRds(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.0")
			testAccPreCheckSkipTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigWithDynamicMySQL8(dbName),
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.test", "CONNECTION_ADMIN", true, false),
					testAccPrivilege("mysql_grant.test", "FIREWALL_EXEMPT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "database", "*"),
					resource.TestCheckResourceAttr("mysql_grant.test", "table", "*"),
				),
			},
		},
	})
}

// Skips RDS, MySQL < 8.0, TiDB (same as original test)
func TestAccGrant_role(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	roleName := fmt.Sprintf("TFRole-exp%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckSkipTiDB(t)
			testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.0")
			testAccPreCheckSkipRds(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigRole(dbName, roleName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant.test", "roles.0", roleName),
				),
			},
			{
				Config: testAccGrantConfigRoleWithGrantOption(dbName, roleName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant.test", "roles.0", roleName),
					resource.TestCheckResourceAttr("mysql_grant.test", "grant", "true"),
				),
			},
			{
				Config: testAccGrantConfigRole(dbName, roleName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant.test", "roles.0", roleName),
				),
			},
		},
	})
}

// Skips RDS, MySQL < 8.0, TiDB (same as original test)
func TestAccGrant_roleToUser(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	roleName := fmt.Sprintf("TFRole-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.0")
			testAccPreCheckSkipRds(t)
			testAccPreCheckSkipTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigRoleToUser(dbName, roleName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant.test", "user", fmt.Sprintf("jdoe-%s", dbName)),
					resource.TestCheckResourceAttr("mysql_grant.test", "host", "example.com"),
					resource.TestCheckResourceAttr("mysql_grant.test", "roles.#", "1"),
				),
			},
		},
	})
}

// Skips MariaDB, MySQL < 8.0, TiDB (same as original test)
func TestAccGrant_complexRoleGrants(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.0")
			testAccPreCheckSkipTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigComplexRoleGrants(dbName),
			},
		},
	})
}

func TestAccGrantOnProcedure(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	procedureName := "test_procedure"
	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))
	userName := fmt.Sprintf("jdoe-%s", dbName)
	hostName := "%"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipTiDB(t) // TiDB doesn't support procedure grants
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				// Create table first
				Config: testAccGrantConfigNoGrant(dbName),
				Check: resource.ComposeTestCheckFunc(
					prepareTable(dbName, "tbl"),
				),
			},
			{
				// Create a procedure
				Config: testAccGrantConfigNoGrant(dbName),
				Check: resource.ComposeTestCheckFunc(
					prepareProcedure(dbName, procedureName),
				),
			},
			{
				Config: testAccGrantConfigProcedureWithTable(procedureName, dbName, hostName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckProcedureGrant("mysql_grant.test_procedure", userName, hostName, procedureName, true),
					resource.TestCheckResourceAttr("mysql_grant.test_procedure", "user", userName),
					resource.TestCheckResourceAttr("mysql_grant.test_procedure", "host", hostName),
				),
			},
		},
	})
}

func TestAllowDuplicateUsersDifferentTables(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))

	duplicateUserConfig := fmt.Sprintf(`
	resource "mysql_database" "test" {
	  name = "%s"
	}

	resource "mysql_user" "test" {
	  user     = "jdoe-%s"
	  host     = "example.com"
	}

	resource "mysql_grant" "grant1" {
	  user       = "${mysql_user.test.user}"
	  host       = "${mysql_user.test.host}"
	  database   = "${mysql_database.test.name}"
      table      = "table1"
	  privileges = ["UPDATE", "SELECT"]
	}

	resource "mysql_grant" "grant2" {
	  user       = "${mysql_user.test.user}"
	  host       = "${mysql_user.test.host}"
	  database   = "${mysql_database.test.name}"
	  table      = "table2"
	  privileges = ["UPDATE", "SELECT"]
	}
	`, dbName, dbName)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				// Create table first
				Config: testAccGrantConfigNoGrant(dbName),
				Check: resource.ComposeTestCheckFunc(
					prepareTable(dbName, "table1"),
					prepareTable(dbName, "table2"),
				),
			},
			{
				Config: duplicateUserConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.grant1", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.grant1", "table", "table1"),
					testAccPrivilege("mysql_grant.grant2", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.grant2", "table", "table2"),
				),
			},
			{
				RefreshState: true,
				Check: resource.ComposeTestCheckFunc(
					testAccPrivilege("mysql_grant.grant1", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.grant1", "table", "table1"),
					testAccPrivilege("mysql_grant.grant2", "SELECT", true, false),
					resource.TestCheckResourceAttr("mysql_grant.grant2", "table", "table2"),
				),
			},
		},
	})
}

func TestDisallowDuplicateUsersSameTable(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	dbName := fmt.Sprintf("tf-test-%d", rand.Intn(100))

	duplicateUserConfig := fmt.Sprintf(`
	resource "mysql_database" "test" {
	  name = "%s"
	}

	resource "mysql_user" "test" {
	  user     = "jdoe-%s"
	  host     = "example.com"
	}

	resource "mysql_grant" "grant1" {
	  user       = "${mysql_user.test.user}"
	  host       = "${mysql_user.test.host}"
	  database   = "${mysql_database.test.name}"
      table      = "table1"
	  privileges = ["UPDATE", "SELECT"]
	}

	resource "mysql_grant" "grant2" {
	  user       = "${mysql_user.test.user}"
	  host       = "${mysql_user.test.host}"
	  database   = "${mysql_database.test.name}"
	  table      = "table1"
	  privileges = ["UPDATE", "SELECT"]
	}
	`, dbName, dbName)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGrantCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGrantConfigNoGrant(dbName),
				Check: resource.ComposeTestCheckFunc(
					prepareTable(dbName, "table1"),
				),
			},
			{
				Config:      duplicateUserConfig,
				ExpectError: regexp.MustCompile("already has"),
			},
		},
	})
}

func testAccGrantConfigBasicWithGrant(dbName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  privileges = ["UPDATE", "SELECT"]
  grant      = "true"
}
`, dbName, dbName)
}

func testAccGrantConfigProcedureWithDatabase(procedureName string, dbName string, hostName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_user" "test_global" {
  user     = "jdoe-%s"
  host     = "%%"
}

resource "mysql_grant" "test_procedure" {
    user       = "jdoe-%s"
    host       = "%s"
    privileges = ["EXECUTE"]
    database   = "PROCEDURE %s.%s"
}
`, dbName, dbName, dbName, dbName, hostName, dbName, procedureName)
}

func testAccGrantConfigBasic(dbName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  privileges = ["UPDATE", "SELECT"]
}
`, dbName, dbName)
}

func testAccPrivilege(rn string, privilege string, expectExists bool, expectGrant bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource not found: %s", rn)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("grant id not set")
		}

		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		id := strings.Split(rs.Primary.ID, ":")

		var userOrRole UserOrRole
		if strings.Contains(id[0], "@") {
			parts := strings.Split(id[0], "@")
			userOrRole = UserOrRole{
				Name: parts[0],
				Host: parts[1],
			}
		} else {
			userOrRole = UserOrRole{
				Name: id[0],
			}
		}

		grants, err := showUserGrants(context.Background(), db, userOrRole)
		if err != nil {
			return err
		}

		privilegeNorm := normalizePerms([]string{privilege})[0]

		var expectedGrant MySQLGrant

	Outer:
		for _, grant := range grants {
			grantWithPrivs, ok := grant.(MySQLGrantWithPrivileges)
			if !ok {
				continue
			}
			for _, priv := range grantWithPrivs.GetPrivileges() {
				log.Printf("[DEBUG] Checking grant %s against %s", priv, privilegeNorm)
				if priv == privilegeNorm {
					expectedGrant = grant
					break Outer
				}
			}
		}

		if expectExists != (expectedGrant != nil) {
			if expectedGrant != nil {
				return fmt.Errorf("grant %s found but it was not requested for %s", privilege, userOrRole)
			} else {
				return fmt.Errorf("grant %s not found for %s", privilegeNorm, userOrRole)
			}
		}

		if expectedGrant != nil && expectedGrant.GrantOption() != expectGrant {
			return fmt.Errorf("grant %s found but had incorrect grant option", privilege)
		}

		// We match expectations.
		return nil
	}
}

func testAccGrantCheckDestroy(s *terraform.State) error {
	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "mysql_grant" {
			continue
		}

		id := strings.Split(rs.Primary.ID, ":")

		var userOrRole string
		if strings.Contains(id[0], "@") {
			parts := strings.Split(id[0], "@")
			userOrRole = fmt.Sprintf("'%s'@'%s'", parts[0], parts[1])
		} else {
			userOrRole = fmt.Sprintf("'%s'", id[0])
		}

		stmtSQL := fmt.Sprintf("SHOW GRANTS FOR %s", userOrRole)
		log.Printf("[DEBUG] SQL: %s", stmtSQL)
		rows, err := db.Query(stmtSQL)
		if err != nil {
			if isNonExistingGrant(err) {
				return nil
			}

			return fmt.Errorf("error reading grant: %s", err)
		}

		if rows.Next() {
			return fmt.Errorf("grant still exists for: %s", userOrRole)
		}
		rows.Close()
	}
	return nil
}

func revokeUserPrivs(dbname string, privs string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		// Revoke privileges for this user
		revokeAllSql := fmt.Sprintf("REVOKE %s ON `%s`.* FROM `jdoe-%s`@`example.com`;", privs, dbname, dbname)
		log.Printf("[DEBUG] SQL: %s", revokeAllSql)
		if _, err := db.Exec(revokeAllSql); err != nil {
			return fmt.Errorf("error revoking grant: %s", err)
		}
		return nil
	}
}

func testAccGrantConfigBroken(dbName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  privileges = ["UPDATE", "SELECT"]
}

resource "mysql_grant" "test2" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  privileges = ["UPDATE", "SELECT"]
}
`, dbName, dbName)
}

func testAccGrantConfigExtraHost(dbName string, extraHost bool) string {
	extra := ""
	if extraHost {
		extra = fmt.Sprintf(`
resource "mysql_grant" "test_bet" {
  user       = "${mysql_user.test_bet.user}"
  host       = "${mysql_user.test_bet.host}"
  database   = "mysql"
  privileges = ["DELETE"]
}
		`)
	}
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test_all" {
  user     = "jdoe-%s"
  host     = "%%"
}

resource "mysql_user" "test" {
  user       = "jdoe-%s"
  host       = "10.1.2.3"
}

resource "mysql_user" "test_bet" {
  user       = "jdoe-%s"
  host       = "10.1.%%.%%"
}

resource "mysql_grant" "test_all" {
  user       = "${mysql_user.test_all.user}"
  host       = "${mysql_user.test_all.host}"
  database   = "mysql"
  privileges = ["UPDATE", "SELECT"]
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "mysql"
  privileges = ["SELECT", "INSERT"]
}
%s
`, dbName, dbName, dbName, dbName, extra)
}

func prepareTable(dbname string, tableName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}
		if _, err := db.Exec(fmt.Sprintf("CREATE TABLE `%s`.`%s`(c1 INT, c2 INT, c3 INT,c4 INT,c5 INT);", dbname, tableName)); err != nil {
			return fmt.Errorf("error reading grant: %s", err)
		}
		return nil
	}
}

func testAccGrantConfigNoGrant(dbName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_user" "test_global" {
  user     = "jdoe-%s"
  host     = "%%"
}

`, dbName, dbName, dbName)
}

func testAccGrantConfigWithPrivs(dbName, privs string, grantOption bool) string {
	grantOptionStr := "false"
	if grantOption {
		grantOptionStr = "true"
	}

	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_user" "test_global" {
  user     = "jdoe-%s"
  host     = "%%"
}

resource "mysql_grant" "test_global" {
  user       = "${mysql_user.test_global.user}"
  host       = "${mysql_user.test_global.host}"
  table      = "*"
  database   = "*"
  privileges = ["SHOW DATABASES"]
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  table      = "tbl"
  database   = "${mysql_database.test.name}"
  privileges = [%s]
  grant      = %s
}
`, dbName, dbName, dbName, privs, grantOptionStr)
}

func testAccGrantConfigWithDynamicMySQL8(dbName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "*"
  privileges = ["CONNECTION_ADMIN", "FIREWALL_EXEMPT"]
}
`, dbName, dbName)
}

func testAccGrantConfigRole(dbName string, roleName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_role" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  roles      = [mysql_role.test.name]
}
`, dbName, roleName, dbName)
}

func testAccGrantConfigRoleWithGrantOption(dbName string, roleName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_role" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_grant" "test" {
  user       = "${mysql_user.test.user}"
  host       = "${mysql_user.test.host}"
  database   = "${mysql_database.test.name}"
  roles      = [mysql_role.test.name]
  grant      = true
}
`, dbName, roleName, dbName)
}

func testAccGrantConfigRoleToUser(dbName string, roleName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "jdoe" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_role" "test" {
  name = "%s"
}

resource "mysql_grant" "test" {
  user     = "${mysql_user.jdoe.user}"
  host     = "${mysql_user.jdoe.host}"
  database = "${mysql_database.test.name}"
  roles    = ["${mysql_role.test.name}"]
}
`, dbName, dbName, roleName)
}

func testAccGrantConfigComplexRoleGrants(user string) string {
	return fmt.Sprintf(`
	locals {
		user = "%v"
		host = "%%"
	}
	resource "mysql_user" "user" {
		user = local.user
		host = local.host
	}
	resource "mysql_role" "role1" {
		name = "role1"
	}
	resource "mysql_role" "role2" {
		name = "role2"
	}
	resource "mysql_grant" "adminuser_roles" {
		user     = mysql_user.user.user
		host     = mysql_user.user.host
		database = "*"
		grant    = true
		roles    = [mysql_role.role1.name, mysql_role.role2.name]
	}
	resource "mysql_grant" "role_perms" {
		role       = mysql_role.role1.name
		database   = "mysql"
		privileges = ["SELECT"]
	}
	resource "mysql_grant" "adminuser_privs" {
		user     = mysql_user.user.user
		host     = mysql_user.user.host
		database   = "*"
		grant      = true
		privileges = ["SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "RELOAD", "PROCESS", "REFERENCES", "INDEX", "ALTER", "SHOW DATABASES", "CREATE TEMPORARY TABLES", "LOCK TABLES", "EXECUTE", "REPLICATION SLAVE", "REPLICATION CLIENT", "CREATE VIEW", "SHOW VIEW", "CREATE ROUTINE", "ALTER ROUTINE", "CREATE USER", "EVENT", "TRIGGER"]
	}`, user)
}

func prepareProcedure(dbname string, procedureName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}
		// Switch to the specified database
		_, err = db.ExecContext(ctx, fmt.Sprintf("USE `%s`", dbname))
		log.Printf("[DEBUG] SQL: %s", dbname)
		if err != nil {
			return fmt.Errorf("error selecting database %s: %s", dbname, err)
		}
		// Check if the procedure exists
		var exists int
		checkExistenceSQL := fmt.Sprintf(`
SELECT COUNT(*)
FROM information_schema.ROUTINES
WHERE ROUTINE_SCHEMA = ? AND ROUTINE_NAME = ? AND ROUTINE_TYPE = 'PROCEDURE'
`)
		log.Printf("[DEBUG] SQL: %s", checkExistenceSQL)
		err = db.QueryRowContext(ctx, checkExistenceSQL, dbname, procedureName).Scan(&exists)
		if err != nil {
			return fmt.Errorf("error checking existence of procedure %s: %s", procedureName, err)
		}
		if exists > 0 {
			return nil
		}
		// Create the procedure
		createProcedureSQL := fmt.Sprintf(`
CREATE PROCEDURE `+"`%s`.`%s`"+`()
BEGIN
    SELECT 1;
END
`, dbname, procedureName)
		log.Printf("[DEBUG] SQL: %s", createProcedureSQL)
		_, err = db.ExecContext(ctx, createProcedureSQL)
		if err != nil {
			return fmt.Errorf("error creating procedure %s: %s", procedureName, err)
		}
		return nil
	}
}

func testAccGrantConfigProcedureWithTable(procedureName string, dbName string, hostName string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}

resource "mysql_user" "test" {
  user     = "jdoe-%s"
  host     = "example.com"
}

resource "mysql_user" "test_global" {
  user     = "jdoe-%s"
  host     = "%%"
}

resource "mysql_grant" "test_procedure" {
    user       = "jdoe-%s"
    host       = "%s"
    privileges = ["EXECUTE"]
    database   = "PROCEDURE %s"
	table 	   = "%s"
}
`, dbName, dbName, dbName, dbName, hostName, dbName, procedureName)
}

func testAccCheckProcedureGrant(resourceName, userName, hostName, procedureName string, expected bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// Obtain the database connection from the Terraform provider
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		// Query to show grants for the specific user
		query := fmt.Sprintf("SHOW GRANTS FOR '%s'@'%s'", userName, hostName)
		log.Printf("[DEBUG] SQL: %s", query)

		// Use db.Query to execute the query
		rows, err := db.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()

		// Variable to track if the required privilege is found
		found := false

		// Iterate through the results
		for rows.Next() {
			var grant string
			if err := rows.Scan(&grant); err != nil {
				return err
			}

			// Check if the grant string contains the necessary privilege
			// Adjust the following line according to the exact format of your GRANT statement
			if strings.Contains(grant, fmt.Sprintf("`%s`", procedureName)) && strings.Contains(grant, "EXECUTE") {
				found = true
				break
			}
		}

		// Check if there was an error during iteration
		if err := rows.Err(); err != nil {
			return err
		}

		// Compare the result with the expected outcome
		if found != expected {
			return fmt.Errorf("grant for procedure %s does not match expected state: %v", procedureName, expected)
		}

		return nil
	}
}
