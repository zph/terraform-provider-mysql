package mysql

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Uses shared container set up in TestMain
func TestAccUserPassword_basic(t *testing.T) {
	// Use shared container set up in TestMain
	_ = getSharedMySQLContainer(t, "")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccUserCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccUserPasswordConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccUserExists("mysql_user.test"),
					resource.TestCheckResourceAttr("mysql_user_password.test", "user", "jdoe"),
					resource.TestCheckResourceAttrSet("mysql_user_password.test", "plaintext_password"),
				),
			},
		},
	})
}
