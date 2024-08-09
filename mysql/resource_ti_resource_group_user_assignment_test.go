package mysql

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestTIDBResourceGroupUserAssignment_basic(t *testing.T) {
	varUsername := "tidb-jdoe"
	varName := "rg100"
	varResourceUnits := 100
	varQueryLimit := "()"
	resourceGroupAssignmentResourceName := "mysql_ti_resource_group_user_assignment.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckSkipNotTiDB(t)
		},
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccResourceGroupUserAssignmentCheckDestroy(varName),
		Steps: []resource.TestStep{
			{
				Config: testAccResourceGroupUserAssignmentBasic(varUsername, varName, varResourceUnits, varQueryLimit),
				Check: resource.ComposeTestCheckFunc(
					testAccResourceGroupUserAssignmentExists(varUsername, varName),
					resource.TestCheckResourceAttr(resourceGroupAssignmentResourceName, "user", varUsername),
					resource.TestCheckResourceAttr(resourceGroupAssignmentResourceName, "resource_group", varName),
				),
			},
		},
	})
}

func testAccResourceGroupUserAssignmentExists(username string, resourceGroupName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		user, resourceGroup, err := readUserFromDB(db, username)
		if err != nil {
			return err
		}

		if user != username {
			return fmt.Errorf("user (%s) does not exist", username)
		}

		if resourceGroup != resourceGroupName {
			return fmt.Errorf("resource group (%s) is not assigned to user (%s)", resourceGroupName, username)
		}

		return nil
	}
}

func testAccResourceGroupUserAssignmentCheckDestroy(varName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		return nil
	}
}

func testAccResourceGroupUserAssignmentBasic(varUsername string, varResourceGroupName string, varResourceUnits int, varQueryLimit string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
	user = "%s"
	host = "%%"
}

resource "mysql_ti_resource_group" "test" {
	name = "%s"
	resource_units = %d
	query_limit = "%s"
}

resource "mysql_ti_resource_group_user_assignment" "test" {
	user = "${mysql_user.test.user}"
	resource_group = "${mysql_ti_resource_group.test.name}"
}
`, varUsername, varResourceGroupName, varResourceUnits, varQueryLimit)
}
