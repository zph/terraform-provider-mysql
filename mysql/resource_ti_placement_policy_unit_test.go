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

func TestPlacementPolicyBuildSQLQueryExtendedTiDBOptions(t *testing.T) {
	pp := PlacementPolicy{
		Name:                "policy1",
		PrimaryRegion:       "us-east-1",
		Regions:             []string{"us-east-1", "us-west-2"},
		Followers:           4,
		Schedule:            "EVEN",
		Learners:            1,
		Constraints:         []string{"+disk=ssd"},
		LeaderConstraints:   []string{"+zone=us-east-1a"},
		FollowerConstraints: []string{"+zone=us-west-2a"},
		LearnerConstraints:  []string{"+zone=us-west-2b"},
		SurvivalPreferences: []string{"region", "zone"},
	}

	got := pp.buildSQLQuery(CreatePlacementPolicySQLPrefix)
	want := `CREATE PLACEMENT POLICY IF NOT EXISTS policy1 PRIMARY_REGION="us-east-1" REGIONS="us-east-1,us-west-2" FOLLOWERS=4 SCHEDULE="EVEN" LEARNERS=1 CONSTRAINTS="[+disk=ssd]" LEADER_CONSTRAINTS="[+zone=us-east-1a]" FOLLOWER_CONSTRAINTS="[+zone=us-west-2a]" LEARNER_CONSTRAINTS="[+zone=us-west-2b]" SURVIVAL_PREFERENCES="[region,zone]" ;`
	if got != want {
		t.Fatalf("buildSQLQuery() = %q, want %q", got, want)
	}
}

func TestPlacementPolicyResourceDataMapping(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceTiPlacementPolicy().Schema, map[string]interface{}{
		"name":                 "policy1",
		"primary_region":       "us-east-1",
		"regions":              []interface{}{"us-east-1", "us-west-2"},
		"followers":            4,
		"schedule":             "EVEN",
		"learners":             1,
		"constraints":          []interface{}{"+zone=us-east-1", "+disk=ssd"},
		"leader_constraints":   []interface{}{"+zone=us-east-1a"},
		"follower_constraints": []interface{}{"+zone=us-west-2a"},
		"learner_constraints":  []interface{}{"+zone=us-west-2b"},
		"survival_preferences": []interface{}{"region", "zone"},
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
	if pp.Followers != 4 {
		t.Fatalf("Followers = %d", pp.Followers)
	}
	if pp.Schedule != "EVEN" {
		t.Fatalf("Schedule = %q", pp.Schedule)
	}
	if pp.Learners != 1 {
		t.Fatalf("Learners = %d", pp.Learners)
	}
	if !reflect.DeepEqual(pp.LeaderConstraints, []string{"+zone=us-east-1a"}) {
		t.Fatalf("LeaderConstraints = %#v", pp.LeaderConstraints)
	}
	if !reflect.DeepEqual(pp.FollowerConstraints, []string{"+zone=us-west-2a"}) {
		t.Fatalf("FollowerConstraints = %#v", pp.FollowerConstraints)
	}
	if !reflect.DeepEqual(pp.LearnerConstraints, []string{"+zone=us-west-2b"}) {
		t.Fatalf("LearnerConstraints = %#v", pp.LearnerConstraints)
	}
	if !reflect.DeepEqual(pp.SurvivalPreferences, []string{"region", "zone"}) {
		t.Fatalf("SurvivalPreferences = %#v", pp.SurvivalPreferences)
	}

	setPlacementPolicyOnResourceData(PlacementPolicy{
		Name:                "policy2",
		PrimaryRegion:       "us-west-2",
		Regions:             []string{"us-west-2"},
		Followers:           2,
		Schedule:            "MAJORITY_IN_PRIMARY",
		Learners:            1,
		Constraints:         []string{"+zone=us-west-2"},
		LeaderConstraints:   []string{"+zone=us-west-2a"},
		FollowerConstraints: []string{"+zone=us-west-2b"},
		LearnerConstraints:  []string{"+zone=us-west-2c"},
		SurvivalPreferences: []string{"zone"},
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
	if got := d.Get("followers").(int); got != 2 {
		t.Fatalf("resource data followers = %d", got)
	}
	if got := d.Get("schedule").(string); got != "MAJORITY_IN_PRIMARY" {
		t.Fatalf("resource data schedule = %q", got)
	}
	if got := d.Get("learners").(int); got != 1 {
		t.Fatalf("resource data learners = %d", got)
	}
	if got := expandStringList(d.Get("leader_constraints").([]interface{})); !reflect.DeepEqual(got, []string{"+zone=us-west-2a"}) {
		t.Fatalf("resource data leader_constraints = %#v", got)
	}
	if got := expandStringList(d.Get("follower_constraints").([]interface{})); !reflect.DeepEqual(got, []string{"+zone=us-west-2b"}) {
		t.Fatalf("resource data follower_constraints = %#v", got)
	}
	if got := expandStringList(d.Get("learner_constraints").([]interface{})); !reflect.DeepEqual(got, []string{"+zone=us-west-2c"}) {
		t.Fatalf("resource data learner_constraints = %#v", got)
	}
	if got := expandStringList(d.Get("survival_preferences").([]interface{})); !reflect.DeepEqual(got, []string{"zone"}) {
		t.Fatalf("resource data survival_preferences = %#v", got)
	}
}

func expandStringList(values []interface{}) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.(string))
	}
	return out
}
