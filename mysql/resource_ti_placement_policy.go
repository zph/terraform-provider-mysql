package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var CreatePlacementPolicySQLPrefix = "CREATE PLACEMENT POLICY IF NOT EXISTS"
var UpdatePlacementPolicySQLPrefix = "ALTER PLACEMENT POLICY"
var BracketsRegex = regexp.MustCompile("^\\[(.+)\\]$")

type PlacementPolicy struct {
	Name                string
	PrimaryRegion       string
	Regions             []string
	Followers           int
	Schedule            string
	Learners            int
	Constraints         []string
	LeaderConstraints   []string
	FollowerConstraints []string
	LearnerConstraints  []string
	SurvivalPreferences []string
}

func (pp *PlacementPolicy) buildSQLQuery(prefix string) string {
	query := []string{}

	baseQuery := fmt.Sprintf(
		"%s %s",
		prefix,
		pp.Name,
	)

	query = append(query, baseQuery)

	if pp.PrimaryRegion != "" {
		primaryRegionClause := fmt.Sprintf(`PRIMARY_REGION="%s"`, pp.PrimaryRegion)
		query = append(query, primaryRegionClause)
	}

	if len(pp.Regions) > 0 {
		regionsClause := fmt.Sprintf(`REGIONS="%s"`, strings.Join(pp.Regions, ","))
		query = append(query, regionsClause)
	}

	if pp.Followers > 0 {
		query = append(query, fmt.Sprintf(`FOLLOWERS=%d`, pp.Followers))
	}

	if pp.Schedule != "" {
		query = append(query, fmt.Sprintf(`SCHEDULE="%s"`, pp.Schedule))
	}

	if pp.Learners > 0 {
		query = append(query, fmt.Sprintf(`LEARNERS=%d`, pp.Learners))
	}

	if len(pp.Constraints) > 0 {
		constraintsClause := fmt.Sprintf(`CONSTRAINTS="[%s]"`, strings.Join(pp.Constraints, ","))
		query = append(query, constraintsClause)
	} else {
		// Allow for empty constraints to be set in order to represent a
		// placement policy without constraints.
		query = append(query, `CONSTRAINTS=""`)
	}

	if len(pp.LeaderConstraints) > 0 {
		query = append(query, fmt.Sprintf(`LEADER_CONSTRAINTS="[%s]"`, strings.Join(pp.LeaderConstraints, ",")))
	}

	if len(pp.FollowerConstraints) > 0 {
		query = append(query, fmt.Sprintf(`FOLLOWER_CONSTRAINTS="[%s]"`, strings.Join(pp.FollowerConstraints, ",")))
	}

	if len(pp.LearnerConstraints) > 0 {
		query = append(query, fmt.Sprintf(`LEARNER_CONSTRAINTS="[%s]"`, strings.Join(pp.LearnerConstraints, ",")))
	}

	if len(pp.SurvivalPreferences) > 0 {
		query = append(query, fmt.Sprintf(`SURVIVAL_PREFERENCES="[%s]"`, strings.Join(pp.SurvivalPreferences, ",")))
	}

	query = append(query, ";")

	ctx := context.Background()
	tflog.SetField(ctx, "sql", query)
	tflog.Debug(ctx, `buildSQLQuery`)

	return strings.Join(query, " ")
}

func resourceTiPlacementPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreatePlacementPolicy,
		ReadContext:   ReadPlacementPolicy,
		UpdateContext: UpdatePlacementPolicy,
		DeleteContext: DeletePlacementPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"primary_region": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "",
			},
			"regions": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			// TiDB placement options follow CREATE/ALTER PLACEMENT POLICY.
			// See https://docs.pingcap.com/tidb/stable/sql-statement-create-placement-policy/.
			"followers": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: false,
			},
			"schedule": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: false,
			},
			"learners": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: false,
			},
			"constraints": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"leader_constraints": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"follower_constraints": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"learner_constraints": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"survival_preferences": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: false,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func CreatePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	pp := NewPlacementPolicyFromResourceData(d)

	var warnLevel, warnMessage string
	var warnCode int = 0

	query := pp.buildSQLQuery(CreatePlacementPolicySQLPrefix)
	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "SQL")

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return diag.Errorf("error creating placement policy (%s): %s", pp.Name, err)
	}

	db.QueryRowContext(ctx, "SHOW WARNINGS").Scan(&warnLevel, &warnCode, &warnMessage)
	if warnCode != 0 {
		return diag.Errorf("error setting value: %+v Error: %s", pp, warnMessage)
	}

	d.SetId(pp.Name)

	return nil
}

func UpdatePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	pp := NewPlacementPolicyFromResourceData(d)

	var warnLevel, warnMessage string
	var warnCode int = 0

	query := pp.buildSQLQuery(UpdatePlacementPolicySQLPrefix)

	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "SQL")

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return diag.Errorf("error altering placement policy (%s): %s", pp.Name, err)
	}

	db.QueryRowContext(ctx, "SHOW WARNINGS").Scan(&warnLevel, &warnCode, &warnMessage)
	if warnCode != 0 {
		return diag.Errorf("error altering placement policy (%s) %s", pp.Name, warnMessage)
	}

	d.SetId(pp.Name)

	return nil
}

func ReadPlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	pp, err := getPlacementPolicyFromDB(db, d.Id())
	if err != nil {
		d.SetId("")
		return diag.Errorf("error during get placement policy (%s): %s", d.Id(), err)
	}

	// If we're not able to find the placement policy, assume that there's terraform
	// diff and allow terraform to recreate it instead of throwing an error.
	if pp == nil {
		d.SetId("")
		return nil
	}

	setPlacementPolicyOnResourceData(*pp, d)
	return nil
}

func DeletePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)

	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	deleteQuery := fmt.Sprintf("DROP PLACEMENT POLICY IF EXISTS %s", name)
	_, err = db.Exec(deleteQuery)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return diag.Errorf("error during drop placement policy (%s): %s", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func NewPlacementPolicyFromResourceData(d *schema.ResourceData) PlacementPolicy {
	return PlacementPolicy{
		Name:                d.Get("name").(string),
		PrimaryRegion:       d.Get("primary_region").(string),
		Regions:             resourceStringList(d, "regions"),
		Followers:           d.Get("followers").(int),
		Schedule:            d.Get("schedule").(string),
		Learners:            d.Get("learners").(int),
		Constraints:         resourceStringList(d, "constraints"),
		LeaderConstraints:   resourceStringList(d, "leader_constraints"),
		FollowerConstraints: resourceStringList(d, "follower_constraints"),
		LearnerConstraints:  resourceStringList(d, "learner_constraints"),
		SurvivalPreferences: resourceStringList(d, "survival_preferences"),
	}
}

func getPlacementPolicyFromDB(db *sql.DB, name string) (*PlacementPolicy, error) {
	pp := PlacementPolicy{Name: name}

	query := `SELECT * FROM information_schema.placement_policies where POLICY_NAME = ?`

	ctx := context.Background()
	tflog.SetField(ctx, "query", query)
	tflog.Debug(ctx, "getPlacementPolicyFromDB")

	row, err := querySingleRowStringMap(db, query, name)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("[DEBUG] placement policy doesn't exist (%s): %s", name, err)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("error during get placement policy (%s): %s", name, err)
	}

	pp.Name = stringMapValue(row, "POLICY_NAME")
	pp.PrimaryRegion = stringMapValue(row, "PRIMARY_REGION")
	pp.Regions = splitCommaString(stringMapValue(row, "REGIONS"))
	pp.Constraints = parsePlacementConstraintList(stringMapValue(row, "CONSTRAINTS"))
	pp.LeaderConstraints = parsePlacementConstraintList(stringMapValue(row, "LEADER_CONSTRAINTS"))
	pp.FollowerConstraints = parsePlacementConstraintList(stringMapValue(row, "FOLLOWER_CONSTRAINTS"))
	pp.LearnerConstraints = parsePlacementConstraintList(stringMapValue(row, "LEARNER_CONSTRAINTS"))
	pp.Schedule = stringMapValue(row, "SCHEDULE")

	if followers, err := stringMapIntValue(row, "FOLLOWERS"); err == nil {
		pp.Followers = followers
	}
	if learners, err := stringMapIntValue(row, "LEARNERS"); err == nil {
		pp.Learners = learners
	}

	// SURVIVAL_PREFERENCES is accepted by CREATE/ALTER PLACEMENT POLICY, but
	// PingCAP's documented information_schema.placement_policies columns do not expose it.
	// See https://docs.pingcap.com/tidb/stable/information-schema-placement-policies/.
	return &pp, nil
}

func splitCommaString(value string) []string {
	if value == "" {
		return nil
	}

	return strings.Split(value, ",")
}

func parsePlacementConstraintList(value string) []string {
	constraintMatches := BracketsRegex.FindStringSubmatch(value)
	if len(constraintMatches) >= 2 {
		return strings.Split(constraintMatches[1], ",")
	}

	if value != "" {
		return []string{value}
	}

	return nil
}

func setPlacementPolicyOnResourceData(pp PlacementPolicy, d *schema.ResourceData) {
	d.Set("name", pp.Name)
	d.Set("primary_region", pp.PrimaryRegion)
	d.Set("regions", pp.Regions)
	d.Set("followers", pp.Followers)
	d.Set("schedule", pp.Schedule)
	d.Set("learners", pp.Learners)
	d.Set("constraints", pp.Constraints)
	d.Set("leader_constraints", pp.LeaderConstraints)
	d.Set("follower_constraints", pp.FollowerConstraints)
	d.Set("learner_constraints", pp.LearnerConstraints)
	if len(pp.SurvivalPreferences) > 0 {
		d.Set("survival_preferences", pp.SurvivalPreferences)
	}
}

func resourceStringList(d *schema.ResourceData, key string) []string {
	valuesAny := d.Get(key).([]any)
	values := []string{}

	for _, valueAny := range valuesAny {
		values = append(values, valueAny.(string))
	}

	return values
}
