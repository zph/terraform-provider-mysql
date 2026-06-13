package mysql

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestPlacementPolicyBuildSQLQuery(t *testing.T) {
	pp := PlacementPolicy{
		Name:          "policy1",
		PrimaryRegion: "us-east-1",
		Regions:       []string{"us-east-1", "us-west-2"},
		Constraints:   []string{"+zone=us-east-1", "+disk=ssd"},
	}

	got := pp.buildSQLQuery(CreatePlacementPolicySQLPrefix)
	want := `CREATE PLACEMENT POLICY IF NOT EXISTS policy1 PRIMARY_REGION="us-east-1" REGIONS="us-east-1,us-west-2" CONSTRAINTS="[+zone=us-east-1,+disk=ssd]" ;`
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestPlacementPolicyBuildSQLQueryEmptyConstraints(t *testing.T) {
	pp := PlacementPolicy{Name: "policy1"}

	got := pp.buildSQLQuery(UpdatePlacementPolicySQLPrefix)
	want := `ALTER PLACEMENT POLICY policy1 CONSTRAINTS="" ;`
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestPlacementPolicyResourceDataMapping(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceTiPlacementPolicy().Schema, map[string]interface{}{
		"name":           "policy1",
		"primary_region": "us-east-1",
		"regions":        []interface{}{"us-east-1", "us-west-2"},
		"constraints":    []interface{}{"+zone=us-east-1", "+disk=ssd"},
	})

	pp := NewPlacementPolicyFromResourceData(d)
	if pp.Name != "policy1" {
		t.Fatalf("Name = %q", pp.Name)
	}
	if pp.PrimaryRegion != "us-east-1" {
		t.Fatalf("PrimaryRegion = %q", pp.PrimaryRegion)
	}
	if !reflect.DeepEqual(pp.Regions, []string{"us-east-1", "us-west-2"}) {
		t.Fatalf("Regions = %#v", pp.Regions)
	}
	if !reflect.DeepEqual(pp.Constraints, []string{"+zone=us-east-1", "+disk=ssd"}) {
		t.Fatalf("Constraints = %#v", pp.Constraints)
	}

	setPlacementPolicyOnResourceData(PlacementPolicy{
		Name:          "policy2",
		PrimaryRegion: "us-west-2",
		Regions:       []string{"us-west-2"},
		Constraints:   []string{"+zone=us-west-2"},
	}, d)

	if got := d.Get("name").(string); got != "policy2" {
		t.Fatalf("resource data name = %q", got)
	}
	if got := d.Get("primary_region").(string); got != "us-west-2" {
		t.Fatalf("resource data primary_region = %q", got)
	}
	if got := expandStringList(d.Get("regions").([]interface{})); !reflect.DeepEqual(got, []string{"us-west-2"}) {
		t.Fatalf("resource data regions = %#v", got)
	}
	if got := expandStringList(d.Get("constraints").([]interface{})); !reflect.DeepEqual(got, []string{"+zone=us-west-2"}) {
		t.Fatalf("resource data constraints = %#v", got)
	}
}

func expandStringList(values []interface{}) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.(string))
	}
	return out
}
