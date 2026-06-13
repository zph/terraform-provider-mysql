package mysql

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceTiPlacementLabels() *schema.Resource {
	return &schema.Resource{
		ReadContext: ShowTiPlacementLabels,
		Schema: map[string]*schema.Schema{
			"labels": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"values": {
							Type:     schema.TypeList,
							Computed: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
		},
	}
}

func ShowTiPlacementLabels(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	// TiDB extension documented at:
	// https://docs.pingcap.com/tidb/stable/sql-statement-show-placement-labels/.
	sql := "SHOW PLACEMENT LABELS"
	log.Printf("[DEBUG] SQL: %s", sql)

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return diag.Errorf("failed querying TiDB placement labels: %v", err)
	}
	defer rows.Close()

	labels := []map[string]interface{}{}
	for rows.Next() {
		var key string
		var rawValues string
		if err := rows.Scan(&key, &rawValues); err != nil {
			return diag.Errorf("failed scanning TiDB placement labels: %v", err)
		}

		values := []string{}
		if err := json.Unmarshal([]byte(rawValues), &values); err != nil {
			values = append(values, rawValues)
		}

		labels = append(labels, map[string]interface{}{
			"key":    key,
			"values": values,
		})
	}
	if err := rows.Err(); err != nil {
		return diag.Errorf("failed reading TiDB placement labels: %v", err)
	}

	if err := d.Set("labels", labels); err != nil {
		return diag.Errorf("failed setting labels field: %v", err)
	}

	d.SetId(id.UniqueId())

	return nil
}
