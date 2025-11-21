package mysql

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
