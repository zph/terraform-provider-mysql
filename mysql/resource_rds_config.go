package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceRDSConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateRDSConfig,
		UpdateContext: UpdateRDSConfig,
		ReadContext:   ReadRDSConfig,
		DeleteContext: DeleteRDSConfig,
		Importer: &schema.ResourceImporter{
			StateContext: ImportRDSConfig,
		},
		Schema: map[string]*schema.Schema{
			"binlog_retention_period": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     0,
				Description: "Sets the number of hours to retain binary log files",
			},
			"replication_target_delay": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     0,
				Description: "Sets the number of seconds to delay replication from source database instance to the read replica",
			},
		},
	}
}

func CreateRDSConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, stmtSQL := range RDSConfigSQL(d) {
		log.Println("Executing statement:", stmtSQL)

		_, err = db.ExecContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed running SQL to set RDS Config: %v", err)
		}
	}

	id := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	d.SetId(id)

	return nil
}

func UpdateRDSConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, stmtSQL := range RDSConfigSQL(d) {
		log.Println("Executing statement:", stmtSQL)

		_, err = db.ExecContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed updating RDS config: %v", err)
		}
	}

	return nil
}

func ReadRDSConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := "call mysql.rds_show_configuration"

	log.Println("Executing query:", stmtSQL)
	rows, err := db.QueryContext(ctx, stmtSQL)
	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			if mysqlErr.Number == unknownDatabaseErrCode {
				d.SetId("")
				return nil
			}
		}
		return diag.Errorf("Error verifying RDS config: %s", err)
	}

	results := make(map[string]string)
	for rows.Next() {
		var name, description string
		var value sql.NullString

		if err := rows.Scan(&name, &value, &description); err != nil {
			return diag.Errorf("failed reading RDS config: %v", err)
		}

		if value.Valid {
			results[name] = value.String
		} else {
			results[name] = "0"
		}
	}

	binlog_retention_period, err := strconv.Atoi(results["binlog retention hours"])
	if err != nil {
		return diag.Errorf("failed reading RDS config: %v", err)
	}
	replication_target_delay, err := strconv.Atoi(results["target delay"])
	if err != nil {
		return diag.Errorf("failed reading RDS config: %v", err)
	}

	d.Set("replication_target_delay", replication_target_delay)
	d.Set("binlog_retention_period", binlog_retention_period)

	return nil
}

func DeleteRDSConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtsSQL := []string{"call mysql.rds_set_configuration('binlog retention hours', NULL)", "call mysql.rds_set_configuration('target delay', 0)"}
	for _, stmtSQL := range stmtsSQL {
		log.Println("Executing statement:", stmtSQL)

		_, err = db.ExecContext(ctx, stmtSQL)
		if err != nil {
			return diag.Errorf("failed unsetting RDS config: %v", err)
		}
	}

	d.SetId("")
	return nil
}

func RDSConfigSQL(d *schema.ResourceData) []string {
	result := []string{}
	if d.Get("binlog_retention_period") != nil {
		retention_period := strconv.Itoa(d.Get("binlog_retention_period").(int))
		if retention_period == "0" {
			retention_period = "NULL"
		}
		result = append(result, (fmt.Sprintf("call mysql.rds_set_configuration('binlog retention hours', %s)", retention_period)))
	}

	if d.Get("replication_target_delay") != nil {
		target_delay := strconv.Itoa(d.Get("replication_target_delay").(int))
		result = append(result, (fmt.Sprintf("call mysql.rds_set_configuration('target delay', %s)", target_delay)))
	}

	return result
}

func ImportRDSConfig(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	id := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	d.SetId(id)

	err := ReadRDSConfig(ctx, d, meta)
	if err != nil {
		return nil, fmt.Errorf("error while importing: %v", err)
	}

	return []*schema.ResourceData{d}, nil
}
