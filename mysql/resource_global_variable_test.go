package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Requires MySQL (not MariaDB/RDS)
// Uses shared container set up in TestMain
// Skips MariaDB, RDS (same as original test)
func TestAccGlobalVar_basic(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	varName := "max_connections"
	resourceName := "mysql_global_variable.test"
	varValue := "1"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSkipMariaDB(t); testAccPreCheckSkipRds(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGlobalVarCheckDestroy(varName, varValue),
		Steps: []resource.TestStep{
			{
				Config: testAccGlobalVarConfigBasic(varName, varValue),
				Check: resource.ComposeTestCheckFunc(
					testAccGlobalVarExists(varName, varValue),
					resource.TestCheckResourceAttr(resourceName, "name", varName),
				),
			},
		},
	})
}

// Requires MySQL (not MariaDB/TiDB/RDS)
// Uses shared container set up in TestMain
// Skips MariaDB, TiDB, RDS (same as original test)
func TestAccGlobalVar_parseBoolean(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	varName := "autocommit"
	resourceName := "mysql_global_variable.test"
	varValue := "OFF"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipNotTiDB(t)
			testAccPreCheckSkipRds(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccGlobalVarCheckDestroy(varName, varValue),
		Steps: []resource.TestStep{
			{
				Config: testAccGlobalVarConfigBasic(varName, varValue),
				Check: resource.ComposeTestCheckFunc(
					testAccGlobalVarExists(varName, varValue),
					resource.TestCheckResourceAttr(resourceName, "name", varName),
				),
			},
		},
	})
}

// Note: TestAccGlobalVar_parseString and TestAccGlobalVar_parseFloat are TiDB-specific
// and require TiDB containers, so they are not converted here.

func testAccGlobalVarExists(varName, varExpected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		res, err := testAccGetGlobalVar(varName, db)

		if err != nil {
			return err
		}

		if res == varExpected {
			return nil
		}

		return fmt.Errorf("variable '%s' not found", varName)
	}
}

func testAccGetGlobalVar(varName string, db *sql.DB) (string, error) {
	stmt, err := db.Prepare("SHOW GLOBAL VARIABLES WHERE VARIABLE_NAME = ?")
	if err != nil {
		return "", err
	}

	var name string
	var value string
	err = stmt.QueryRow(varName).Scan(&name, &value)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	return value, nil
}

func testAccGlobalVarCheckDestroy(varName, varExpected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		res, _ := testAccGetGlobalVar(varName, db)
		if res == varExpected {
			return fmt.Errorf("global variable '%s' still has non default value", varName)
		}

		return nil
	}
}

func testAccGlobalVarConfigBasic(varName, varValue string) string {
	return fmt.Sprintf(`
resource "mysql_global_variable" "test" {
  name = "%s"
	value = "%s"
}
`, varName, varValue)
}
