package mysql

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccGlobalVar_basic(t *testing.T) {
	varName := "max_connections"
	resourceName := "mysql_global_variable.test"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccGlobalVarCheckDestroy(varName),
		Steps: []resource.TestStep{
			{
				Config: testAccGlobalVarConfig_basic(varName),
				Check: resource.ComposeTestCheckFunc(
					testAccGlobalVarExists(varName),
					resource.TestCheckResourceAttr(resourceName, "name", varName),
				),
			},
		},
	})
}

func testAccGlobalVarExists(varName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		db, err := connectToMySQL(testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		count, err := testAccGetGlobalVar(varName, db)

		if err != nil {
			return err
		}

		if count == 1 {
			return nil
		}

		return fmt.Errorf("variable '%s' not found", varName)
	}
}

func testAccGetGlobalVar(varName string, db *sql.DB) (int, error) {
	stmt, err := db.Prepare("SHOW GLOBAL VARIABLES WHERE VARIABLE_NAME = ?")
	if err != nil {
		return 0, err
	}

	var name string
	var value int
	err = stmt.QueryRow(varName).Scan(&name, &value)

	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	return value, nil
}

func testAccGlobalVarCheckDestroy(varName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		db, err := connectToMySQL(testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		count, err := testAccGetGlobalVar(varName, db)
		if count == 1 {
			return fmt.Errorf("Global variable '%s' still has non default value", varName)
		}

		return nil
	}
}

func testAccGlobalVarConfig_basic(varName string) string {
	return fmt.Sprintf(`
resource "mysql_global_variable" "test" {
  name = "%s"
	value = 1
}
`, varName)
}
