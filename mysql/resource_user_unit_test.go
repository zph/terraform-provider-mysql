package mysql

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestTiDBVersionSupportsMaxUserConnections(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "8.5.4", want: false},
		{version: "8.5.5", want: true},
		{version: "v8.5.5", want: true},
		{version: "8.5.6", want: true},
		{version: "9.0.0", want: true},
	}

	for _, tt := range tests {
		got, err := tidbVersionSupportsMaxUserConnections(tt.version)
		if err != nil {
			t.Fatalf("tidbVersionSupportsMaxUserConnections(%q) returned error: %s", tt.version, err)
		}
		if got != tt.want {
			t.Fatalf("tidbVersionSupportsMaxUserConnections(%q) = %t, want %t", tt.version, got, tt.want)
		}
	}
}

func TestParseWithClauseSetting(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceUser().Schema, map[string]interface{}{
		"user":                 "app",
		"max_user_connections": 10,
		"max_statement_time":   1.5,
	})

	parseWithClauseSetting(d, "MAX_USER_CONNECTIONS 25 MAX_STATEMENT_TIME 2.75", "max_user_connections", "MAX_USER_CONNECTIONS", false)
	parseWithClauseSetting(d, "MAX_USER_CONNECTIONS 25 MAX_STATEMENT_TIME 2.75", "max_statement_time", "MAX_STATEMENT_TIME", true)

	if got := d.Get("max_user_connections").(int); got != 25 {
		t.Fatalf("max_user_connections = %d, want 25", got)
	}
	if got := d.Get("max_statement_time").(float64); got != 2.75 {
		t.Fatalf("max_statement_time = %f, want 2.75", got)
	}
}

func TestRedactCreateUserArgs(t *testing.T) {
	got := redactCreateUserArgs([]interface{}{"app", "%", "password", "hash"}, "password", "hash")
	want := []interface{}{"app", "%", "<SENSITIVE>", "<SENSITIVE>"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("redactCreateUserArgs()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
