package mysql

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestResourceGroupBuildSQLQuery(t *testing.T) {
	rg := ResourceGroup{
		Name:          "rg100",
		ResourceUnits: 2000000,
		Priority:      "HIGH",
		Burstable:     true,
		QueryLimit:    "EXEC_ELAPSED='15s', ACTION=COOLDOWN, WATCH=SIMILAR DURATION='10m0s'",
	}

	got := rg.buildSQLQuery(UpdateResourceGroupSQLPrefix)
	want := "ALTER RESOURCE GROUP rg100 RU_PER_SEC = 2000000 PRIORITY = HIGH QUERY_LIMIT=(EXEC_ELAPSED='15s', ACTION=COOLDOWN, WATCH=SIMILAR DURATION='10m0s') BURSTABLE = true ;"
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestResourceGroupBuildSQLQueryEmptyQueryLimit(t *testing.T) {
	rg := ResourceGroup{
		Name:          "rg100",
		ResourceUnits: 100,
		Priority:      "LOW",
		Burstable:     false,
	}

	got := rg.buildSQLQuery(CreateResourceGroupSQLPrefix)
	want := "CREATE RESOURCE GROUP IF NOT EXISTS rg100 RU_PER_SEC = 100 PRIORITY = LOW QUERY_LIMIT=NULL BURSTABLE = false ;"
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestResourceGroupBuildSQLQueryRunawayQueryControls(t *testing.T) {
	tests := []struct {
		name       string
		queryLimit string
		want       string
	}{
		{
			name:       "dry run elapsed time",
			queryLimit: "EXEC_ELAPSED='5s', ACTION=DRYRUN",
			want:       "ALTER RESOURCE GROUP rg_runaway RU_PER_SEC = 5000 PRIORITY = MEDIUM QUERY_LIMIT=(EXEC_ELAPSED='5s', ACTION=DRYRUN) BURSTABLE = false ;",
		},
		{
			name:       "processed keys and ru switch group",
			queryLimit: "PROCESSED_KEYS=1000000, RU=2000, ACTION=SWITCH_GROUP(rg_quarantine), WATCH=PLAN DURATION='30m'",
			want:       "ALTER RESOURCE GROUP rg_runaway RU_PER_SEC = 5000 PRIORITY = MEDIUM QUERY_LIMIT=(PROCESSED_KEYS=1000000, RU=2000, ACTION=SWITCH_GROUP(rg_quarantine), WATCH=PLAN DURATION='30m') BURSTABLE = false ;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rg := ResourceGroup{
				Name:          "rg_runaway",
				ResourceUnits: 5000,
				Priority:      "MEDIUM",
				Burstable:     false,
				QueryLimit:    tt.queryLimit,
			}

			if got := rg.buildSQLQuery(UpdateResourceGroupSQLPrefix); got != tt.want {
				t.Fatalf("buildSQLQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceGroupBuildSQLQueryBackground(t *testing.T) {
	rg := ResourceGroup{
		Name:          "default",
		ResourceUnits: 2147483647,
		Priority:      "MEDIUM",
		Burstable:     true,
		Background:    `TASK_TYPES='br,ddl', UTILIZATION_LIMIT=30`,
	}

	got := rg.buildSQLQuery(UpdateResourceGroupSQLPrefix)
	want := "ALTER RESOURCE GROUP default RU_PER_SEC = 2147483647 PRIORITY = MEDIUM QUERY_LIMIT=NULL BACKGROUND=(TASK_TYPES='br,ddl', UTILIZATION_LIMIT=30) BURSTABLE = true ;"
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestResourceGroupParsesTiDBResourceGroupReadValues(t *testing.T) {
	resourceUnits, err := parseResourceGroupResourceUnits("UNLIMITED")
	if err != nil {
		t.Fatalf("parseResourceGroupResourceUnits() error = %v", err)
	}
	if resourceUnits != 2147483647 {
		t.Fatalf("resourceUnits = %d, want 2147483647", resourceUnits)
	}

	burstable := parseResourceGroupBurstable("YES")
	if !burstable {
		t.Fatal("burstable = false, want true for YES")
	}

	burstable = parseResourceGroupBurstable("NO")
	if burstable {
		t.Fatal("burstable = true, want false for NO")
	}
}

func TestResourceGroupResourceDataMapping(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceTiResourceGroup().Schema, map[string]interface{}{
		"name":           "rg100",
		"resource_units": 100,
		"priority":       "low",
		"burstable":      true,
		"query_limit":    "",
	})

	rg := NewResourceGroupFromResourceData(d)
	if rg.Name != "rg100" {
		t.Fatalf("Name = %q", rg.Name)
	}
	if rg.ResourceUnits != 100 {
		t.Fatalf("ResourceUnits = %d", rg.ResourceUnits)
	}
	if rg.Priority != "LOW" {
		t.Fatalf("Priority = %q, want LOW", rg.Priority)
	}
	if !rg.Burstable {
		t.Fatal("Burstable = false, want true")
	}
	if rg.QueryLimit != "" {
		t.Fatalf("QueryLimit = %q", rg.QueryLimit)
	}

	setResourceGroupOnResourceData(ResourceGroup{
		Name:          "rg200",
		ResourceUnits: 200,
		Priority:      "medium",
		Burstable:     false,
		QueryLimit:    "EXEC_ELAPSED='15s'",
	}, d)

	if got := d.Get("name").(string); got != "rg200" {
		t.Fatalf("resource data name = %q", got)
	}
	if got := d.Get("resource_units").(int); got != 200 {
		t.Fatalf("resource data resource_units = %d", got)
	}
	if got := d.Get("priority").(string); got != "medium" {
		t.Fatalf("resource data priority = %q", got)
	}
	if got := d.Get("burstable").(bool); got {
		t.Fatal("resource data burstable = true, want false")
	}
	if got := d.Get("query_limit").(string); got != "EXEC_ELAPSED='15s'" {
		t.Fatalf("resource data query_limit = %q", got)
	}
}
