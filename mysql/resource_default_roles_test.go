package mysql

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Uses shared container set up in TestMain (MySQL 8.0 required for default roles)
// Skips MySQL < 8.0, MariaDB, TiDB (same as original test)
func TestAccDefaultRoles_basic(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipNotMySQL8(t)
			testAccPreCheckSkipMariaDB(t)
			testAccPreCheckSkipTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccDefaultRolesCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDefaultRolesBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccDefaultRoles("mysql_default_roles.test", "role1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.#", "1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.0", "role1"),
				),
			},
			{
				Config: testAccDefaultRolesMultiple,
				Check: resource.ComposeTestCheckFunc(
					testAccDefaultRoles("mysql_default_roles.test", "role1", "role2"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.#", "2"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.0", "role1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.1", "role2"),
				),
			},
			{
				Config: testAccDefaultRolesNone,
				Check: resource.ComposeTestCheckFunc(
					testAccDefaultRoles("mysql_default_roles.test"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "roles.#", "0"),
				),
			},
			{
				Config:            testAccDefaultRolesBasic,
				ResourceName:      "mysql_default_roles.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%v@%v", "jdoe", "%"),
			},
			{
				Config:            testAccDefaultRolesMultiple,
				ResourceName:      "mysql_default_roles.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%v@%v", "jdoe", "%"),
			},
		},
	})
}
