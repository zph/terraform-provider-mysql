package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type ResourceGroup struct {
	Name          string
	ResourceUnits int
	Priority      string
	Burstable     bool
	BurstableMode string
	QueryLimit    string
	Background    string
}

const (
	ResourceGroupBurstableModeOff       = "off"
	ResourceGroupBurstableModeModerated = "moderated"
	ResourceGroupBurstableModeUnlimited = "unlimited"
)

var CreateResourceGroupSQLPrefix = "CREATE RESOURCE GROUP IF NOT EXISTS"
var UpdateResourceGroupSQLPrefix = "ALTER RESOURCE GROUP"

func (rg *ResourceGroup) buildSQLQuery(prefix string) string {
	var query []string
	baseQuery := fmt.Sprintf("%s %s RU_PER_SEC = %d", prefix, rg.Name, rg.ResourceUnits)
	query = append(query, baseQuery)

	query = append(query, fmt.Sprintf(`PRIORITY = %s`, rg.Priority))

	if rg.QueryLimit == "" {
		query = append(query, fmt.Sprintf(`QUERY_LIMIT=NULL`))
	} else {
		query = append(query, fmt.Sprintf(`QUERY_LIMIT=(%s)`, rg.QueryLimit))
	}

	if rg.Background != "" {
		query = append(query, rg.backgroundSQLClause())
	}

	query = append(query, rg.burstableSQLClause())
	query = append(query, ";")

	ctx := context.TODO()
	tflog.SetField(ctx, "sql", query)
	tflog.Debug(ctx, `buildSQLQuery`)
	return strings.Join(query, " ")
}

func (rg *ResourceGroup) burstableSQLClause() string {
	if rg.BurstableMode != "" {
		return fmt.Sprintf(`BURSTABLE = %s`, strings.ToUpper(rg.BurstableMode))
	}

	return fmt.Sprintf(`BURSTABLE = %t`, rg.Burstable)
}

func (rg *ResourceGroup) backgroundSQLClause() string {
	if strings.EqualFold(strings.TrimSpace(rg.Background), "NULL") {
		return "BACKGROUND=NULL"
	}

	return fmt.Sprintf(`BACKGROUND=(%s)`, rg.Background)
}

var DefaultResourceGroup = ResourceGroup{
	Name:       "tfDefault",
	Priority:   "medium",
	Burstable:  false,
	QueryLimit: "",
}

var ResourceGroupTiDBMinVersion = "7.5.0"

func resourceTiResourceGroup() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateResourceGroup,
		ReadContext:   ReadResourceGroup,
		UpdateContext: UpdateResourceGroup,
		DeleteContext: DeleteResourceGroup,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			// TODO: allow a centralized way to check if there's capacity remaining to use
			"resource_units": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"priority": {
				Type:         schema.TypeString,
				Default:      DefaultResourceGroup.Priority,
				ForceNew:     false,
				ValidateFunc: validation.StringInSlice([]string{"high", "medium", "low"}, false),
				Optional:     true,
			},
			"burstable": {
				Type:     schema.TypeBool,
				Default:  DefaultResourceGroup.Burstable,
				ForceNew: false,
				Optional: true,
			},
			// TiDB v9.0 extends BURSTABLE from a boolean into OFF/MODERATED/UNLIMITED modes.
			// See https://docs.pingcap.com/tidb/stable/sql-statement-create-resource-group/.
			"burstable_mode": {
				Type:         schema.TypeString,
				ForceNew:     false,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.StringInSlice([]string{ResourceGroupBurstableModeOff, ResourceGroupBurstableModeModerated, ResourceGroupBurstableModeUnlimited}, false),
			},
			/*
				QUERY_LIMIT=(EXEC_ELAPSED='60s', ACTION=KILL, WATCH=EXACT DURATION='10m')
			*/
			"query_limit": {
				Type:     schema.TypeString,
				Default:  DefaultResourceGroup.QueryLimit,
				ForceNew: false,
				Optional: true,
			},
			// TiDB exposes BACKGROUND on RESOURCE GROUP for the default group only.
			// See https://docs.pingcap.com/tidb/stable/tidb-resource-control-background-tasks/.
			"background": {
				Type:     schema.TypeString,
				ForceNew: false,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func CreateResourceGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rg := NewResourceGroupFromResourceData(d)

	var warnLevel, warnMessage string
	var warnCode int = 0

	query := rg.buildSQLQuery(CreateResourceGroupSQLPrefix)
	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "SQL")

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return diag.Errorf("error creating resource group (%s): %s", rg.Name, err)
	}

	db.QueryRowContext(ctx, "SHOW WARNINGS").Scan(&warnLevel, &warnCode, &warnMessage)
	if warnCode != 0 {
		return diag.Errorf("error setting value: %+v Error: %s", rg, warnMessage)
	}

	d.SetId(rg.Name)

	return nil
}

func UpdateResourceGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rg := NewResourceGroupFromResourceData(d)

	var warnLevel, warnMessage string
	var warnCode int = 0

	query := rg.buildSQLQuery(UpdateResourceGroupSQLPrefix)

	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "SQL")

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return diag.Errorf("error altering resource group (%s): %s", rg.Name, err)
	}

	db.QueryRowContext(ctx, "SHOW WARNINGS").Scan(&warnLevel, &warnCode, &warnMessage)
	if warnCode != 0 {
		return diag.Errorf("error setting value: %s -> %d Error: %s", rg.Name, rg.ResourceUnits, warnMessage)
	}

	d.SetId(rg.Name)

	return nil
}

func ReadResourceGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	rg, err := getResourceGroupFromDB(db, d.Id())
	if err != nil {
		d.SetId("")
		return diag.Errorf("error during get resource group (%s): %s", d.Id(), err)
	}

	// If we're not able to find the resource group, assume that there's terraform
	// diff and allow terraform to recreate it instead of throwing an error.
	if rg == nil {
		d.SetId("")
		return nil
	}

	setResourceGroupOnResourceData(*rg, d)
	return nil
}

func DeleteResourceGroup(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)

	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	deleteQuery := fmt.Sprintf("DROP RESOURCE GROUP IF EXISTS %s", name)
	_, err = db.Exec(deleteQuery)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return diag.Errorf("error during drop resource group (%s): %s", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func getResourceGroupFromDB(db *sql.DB, name string) (*ResourceGroup, error) {
	rg := ResourceGroup{Name: name}

	/*
		TiDB has changed information_schema.resource_groups across resource-control releases:
		RU_PER_SEC can be numeric or UNLIMITED, BURSTABLE can be YES/NO or a v9 mode,
		and BACKGROUND only exists on newer versions.
		See https://docs.pingcap.com/tidb/stable/sql-statement-create-resource-group/.
	*/
	query := `SELECT * FROM information_schema.resource_groups WHERE NAME = ?`

	ctx := context.Background()
	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "getResourceGroupFromDB")

	row, err := querySingleRowStringMap(db, query, name)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("[DEBUG] resource group doesn't exist (%s): %s", name, err)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("error during get resource group (%s): %s", name, err)
	}

	rg.Name = stringMapValue(row, "NAME")
	rg.Priority = strings.ToLower(stringMapValue(row, "PRIORITY"))
	rg.QueryLimit = stringMapValue(row, "QUERY_LIMIT")
	rg.Background = stringMapValue(row, "BACKGROUND")

	resourceUnits, err := parseResourceGroupResourceUnits(stringMapValue(row, "RU_PER_SEC"))
	if err != nil {
		return nil, fmt.Errorf("error parsing resource group RU_PER_SEC (%s): %s", name, err)
	}
	rg.ResourceUnits = resourceUnits

	rg.Burstable, rg.BurstableMode = parseResourceGroupBurstable(stringMapValue(row, "BURSTABLE"))

	return &rg, nil
}

func NewResourceGroupFromResourceData(d *schema.ResourceData) ResourceGroup {
	burstableMode := ""
	if rawMode, ok := d.GetOk("burstable_mode"); ok {
		burstableMode = strings.ToLower(rawMode.(string))
	}

	return ResourceGroup{
		Name:          d.Get("name").(string),
		ResourceUnits: d.Get("resource_units").(int),
		Priority:      strings.ToUpper(d.Get("priority").(string)),
		Burstable:     d.Get("burstable").(bool),
		BurstableMode: burstableMode,
		QueryLimit:    d.Get("query_limit").(string),
		Background:    d.Get("background").(string),
	}
}

func setResourceGroupOnResourceData(rg ResourceGroup, d *schema.ResourceData) {
	d.Set("name", rg.Name)
	d.Set("resource_units", rg.ResourceUnits)
	d.Set("priority", rg.Priority)
	d.Set("burstable", rg.Burstable)
	d.Set("burstable_mode", rg.BurstableMode)
	d.Set("query_limit", rg.QueryLimit)
	d.Set("background", rg.Background)
}

func parseResourceGroupResourceUnits(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if strings.EqualFold(value, "UNLIMITED") {
		return math.MaxInt32, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}

	return parsed, nil
}

func parseResourceGroupBurstable(raw string) (bool, string) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "YES", "TRUE", "1":
		return true, ""
	case "NO", "FALSE", "0":
		return false, ""
	case "OFF":
		return false, ResourceGroupBurstableModeOff
	case "MODERATED":
		return true, ResourceGroupBurstableModeModerated
	case "UNLIMITED":
		return true, ResourceGroupBurstableModeUnlimited
	default:
		return false, ""
	}
}
