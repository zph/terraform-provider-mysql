package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceTiPlacementRangePolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateOrUpdatePlacementRangePolicy,
		ReadContext:   ReadPlacementRangePolicy,
		UpdateContext: CreateOrUpdatePlacementRangePolicy,
		DeleteContext: DeletePlacementRangePolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			// TiDB ALTER RANGE supports only global and meta.
			// See https://docs.pingcap.com/tidb/stable/sql-statement-alter-range/.
			"range": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"global", "meta"}, false),
			},
			"placement_policy": {
				Type:     schema.TypeString,
				Required: true,
			},
			"placement": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"scheduling_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func CreateOrUpdatePlacementRangePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rangeName := strings.ToLower(d.Get("range").(string))
	placementPolicy := d.Get("placement_policy").(string)

	query := fmt.Sprintf("ALTER RANGE %s PLACEMENT POLICY = %s", rangeName, quoteSQLString(placementPolicy))
	log.Printf("[DEBUG] SQL: %s", query)

	if _, err := db.ExecContext(ctx, query); err != nil {
		return diag.Errorf("error setting TiDB placement policy (%s) on range (%s): %s", placementPolicy, rangeName, err)
	}

	d.SetId(rangeName)

	return ReadPlacementRangePolicy(ctx, d, meta)
}

func ReadPlacementRangePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rangeName := strings.ToLower(d.Get("range").(string))
	if rangeName == "" {
		rangeName = strings.ToLower(d.Id())
	}

	placement, schedulingState, err := readPlacementRangePolicyFromDB(ctx, db, rangeName)
	if err != nil {
		return diag.Errorf("error reading TiDB placement range policy (%s): %s", rangeName, err)
	}

	d.Set("range", rangeName)
	d.Set("placement", placement)
	d.Set("scheduling_state", schedulingState)
	d.SetId(rangeName)

	return nil
}

func DeletePlacementRangePolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rangeName := strings.ToLower(d.Get("range").(string))
	query := fmt.Sprintf("ALTER RANGE %s PLACEMENT POLICY = %s", rangeName, quoteSQLString("default"))
	log.Printf("[DEBUG] SQL: %s", query)

	if _, err := db.ExecContext(ctx, query); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return diag.Errorf("error resetting TiDB placement range policy (%s): %s", rangeName, err)
	}

	d.SetId("")

	return nil
}

func readPlacementRangePolicyFromDB(ctx context.Context, db *sql.DB, rangeName string) (string, string, error) {
	// TiDB SHOW PLACEMENT expands range policy placement instead of returning the policy name.
	// See https://docs.pingcap.com/tidb/stable/placement-rules-in-sql/.
	rows, err := db.QueryContext(ctx, "SHOW PLACEMENT")
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	target := tiDBPlacementRangeTarget(rangeName)
	for rows.Next() {
		var rowTarget string
		var placement string
		var schedulingState string
		if err := rows.Scan(&rowTarget, &placement, &schedulingState); err != nil {
			return "", "", err
		}

		if rowTarget == target {
			return placement, schedulingState, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}

	return "", "", nil
}

func tiDBPlacementRangeTarget(rangeName string) string {
	switch strings.ToLower(rangeName) {
	case "global":
		return "RANGE TiDB_GLOBAL"
	case "meta":
		return "RANGE TiDB_META"
	default:
		return "RANGE " + rangeName
	}
}
