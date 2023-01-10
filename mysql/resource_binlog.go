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

func resourceBinLog() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateBinLog,
		UpdateContext: UpdateBinLog,
		ReadContext:   ReadBinLog,
		DeleteContext: DeleteBinLog,
		Importer: &schema.ResourceImporter{
			StateContext: ImportDatabase,
		},
		Schema: map[string]*schema.Schema{
			"retention_period": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "NULL",
			},
		},
	}
}

func CreateBinLog(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := binlogConfigSQL(d)
	log.Println("Executing statement:", stmtSQL)

	_, err = db.ExecContext(ctx, stmtSQL)
	if err != nil {
		return diag.Errorf("failed running SQL to set binlog retention period: %v", err)
	}

	id := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	d.SetId(id)

	return ReadBinLog(ctx, d, meta)
}

func UpdateBinLog(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := binlogConfigSQL(d)
	log.Println("Executing statement:", stmtSQL)

	_, err = db.ExecContext(ctx, stmtSQL)
	if err != nil {
		return diag.Errorf("failed updating binlog retention period: %v", err)
	}

	return ReadBinLog(ctx, d, meta)
}

func ReadBinLog(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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
		return diag.Errorf("Error verifying binlog retention period: %s", err)
	}

	results := make(map[string]interface{})
	for rows.Next() {
		var name, description string
		var value sql.NullString

		if err := rows.Scan(&name, &value, &description); err != nil {
			return diag.Errorf("failed reading binlog retention period: %v", err)
		}
		results[name] = value
	}
	if results["binlog retention hours"] == "NULL" {
		results["binlog retention hours"] = "0"
	}

	d.Set("retention_period", results["binlog retention hours"])

	return nil
}

func DeleteBinLog(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	stmtSQL := "call mysql.rds_set_configuration('binlog retention hours', NULL)"
	log.Println("Executing statement:", stmtSQL)

	_, err = db.ExecContext(ctx, stmtSQL)
	if err != nil {
		return diag.Errorf("failed unsetting binlog retention period: %v", err)
	}

	d.SetId("")
	return nil
}

func binlogConfigSQL(d *schema.ResourceData) string {
	retention_period := d.Get("retention_period").(string)
	if retention_period == "0" {
		return fmt.Sprintf(
			"call mysql.rds_set_configuration('binlog retention hours', %s)",
			"NULL")
	} else {
		return fmt.Sprintf(
			"call mysql.rds_set_configuration('binlog retention hours', %s)",
			retention_period,
		)
	}
}
