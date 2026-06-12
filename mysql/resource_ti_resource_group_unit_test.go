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
