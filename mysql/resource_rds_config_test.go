package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccResourceRDS(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	binlog := acctest.RandIntRange(0, 78)
	targetDelay := acctest.RandIntRange(0, 7200)
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccRDSConfig_basic(rName, binlog, targetDelay),
				Check: resource.ComposeTestCheckFunc(
					testAccRDSConfigExists(fmt.Sprintf("mysql_rds_config.%s", rName)),
					resource.TestCheckResourceAttr(fmt.Sprintf("mysql_rds_config.%s", rName), "binlog_retention_period", fmt.Sprintf("%d", binlog)),
					resource.TestCheckResourceAttr(fmt.Sprintf("mysql_rds_config.%s", rName), "replication_target_delay", fmt.Sprintf("%d", targetDelay)),
				),
			},
		},
	})
}

func testAccRDSConfig_basic(rName string, binlog int, replication int) string {
	return fmt.Sprintf(`
resource "mysql_rds_config" "%s" {
                binlog_retention_period = %d
                replication_target_delay = %d
}`, rName, binlog, replication)
}

func testAccRDSConfigExists(rn string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource not found: %s", rn)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("RDS config id not set")
		}

		return nil
	}
}

func testAccRDSCheckDestroy() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		stmtSQL := "call mysql.rds_show_configuration"

		log.Println("Executing query:", stmtSQL)
		rows, err := db.QueryContext(ctx, stmtSQL)
		if err != nil {
			return err
		}

		results := make(map[string]string)
		for rows.Next() {
			var name, description string
			var value sql.NullString

			if err := rows.Scan(&name, &value, &description); err != nil {
				return fmt.Errorf("failed reading RDS config: %v", err)
			}

			if value.Valid {
				results[name] = value.String
			} else {
				results[name] = "0"
			}

			if results[name] != "0" {
				return fmt.Errorf("binlog/targetDelay still set after destroy")
			}
		}
		return nil
	}
}

func TestAccResourceRDSConfigChange(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	fullResourceName := fmt.Sprintf("mysql_rds_config.%s", rName)
	binlog := acctest.RandIntRange(0, 72)
	binlogUpdated := acctest.RandIntRange(73, 96)
	targetDelay := acctest.RandIntRange(0, 7200)
	targetDelayUpdated := acctest.RandIntRange(7201, 8000)

	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccRDSCheckDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccRDSConfig_basic(rName, binlog, targetDelay),
				Check: resource.ComposeTestCheckFunc(
					testAccRDSConfigExists(fmt.Sprintf("mysql_rds_config.%s", rName)),
				),
			},
			{
				ResourceName: fullResourceName,
				ImportState:  true,
				PreConfig: func() {
					db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
					if err != nil {
						return
					}

					_, err = db.QueryContext(ctx, fmt.Sprintf("call mysql.rds_set_configuration('binlog retention hours', %d)", binlogUpdated))
					if err != nil {
						fmt.Errorf("%v", err)
					}

					_, err = db.QueryContext(ctx, fmt.Sprintf("call mysql.rds_set_configuration('target delay', %d)", targetDelayUpdated))
					if err != nil {
						fmt.Errorf("%v", err)
					}
				},
				Check: resource.ComposeTestCheckFunc(
					testAccRDSConfigExists(fmt.Sprintf("mysql_rds_config.%s", rName)),
					testAccRDSCheck_full(fullResourceName, binlog, targetDelay),
				),
			},
		},
	})
}

func testAccRDSCheck_full(rn string, binlog int, targetDelay int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource not found: %s", rn)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("RDS config id not set")
		}

		ctx := context.Background()
		db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
		if err != nil {
			return err
		}

		stmtSQL := "call mysql.rds_show_configuration"

		rows, err := db.QueryContext(ctx, stmtSQL)
		if err != nil {
			return err
		}

		results := make(map[string]string)
		for rows.Next() {
			var name, description string
			var value sql.NullString

			if err := rows.Scan(&name, &value, &description); err != nil {
				return fmt.Errorf("failed reading RDS config: %v", err)
			}

			if value.Valid {
				results[name] = value.String
			} else {
				results[name] = "0"
			}
		}

		binlog_retention_period, err := strconv.Atoi(results["binlog retention hours"])
		if err != nil {
			return fmt.Errorf("failed reading RDS config: %v", err)
		}
		replication_target_delay, err := strconv.Atoi(results["target delay"])
		if err != nil {
			return fmt.Errorf("failed reading RDS config: %v", err)
		}

		if binlog_retention_period == binlog {
			return fmt.Errorf("binlog retention should NOT be %d", binlog)
		}

		if replication_target_delay == targetDelay {
			return fmt.Errorf("target delay should NOT be %d", replication_target_delay)
		}

		return nil
	}
}
