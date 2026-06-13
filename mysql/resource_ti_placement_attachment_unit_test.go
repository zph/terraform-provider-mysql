package mysql

import "testing"

func TestTiDBPlacementPolicyClause(t *testing.T) {
	tests := map[string]string{
		"default":       "default",
		"DEFAULT":       "default",
		"regional":      "`regional`",
		"regional-prod": "`regional-prod`",
	}

	for input, want := range tests {
		if got := tiDBPlacementPolicyClause(input); got != want {
			t.Fatalf("tiDBPlacementPolicyClause(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTiDBPlacementPolicyIDParsing(t *testing.T) {
	database, err := parseTiDatabasePlacementPolicyID("app")
	if err != nil {
		t.Fatalf("parseTiDatabasePlacementPolicyID returned error: %s", err)
	}
	if database != "app" {
		t.Fatalf("database = %q, want app", database)
	}

	database, table, err := parseTiTablePlacementPolicyID("app.orders")
	if err != nil {
		t.Fatalf("parseTiTablePlacementPolicyID returned error: %s", err)
	}
	if database != "app" || table != "orders" {
		t.Fatalf("table ID parsed as %q.%q, want app.orders", database, table)
	}

	database, table, partition, err := parseTiPartitionPlacementPolicyID("app.orders.p0")
	if err != nil {
		t.Fatalf("parseTiPartitionPlacementPolicyID returned error: %s", err)
	}
	if database != "app" || table != "orders" || partition != "p0" {
		t.Fatalf("partition ID parsed as %q.%q.%q, want app.orders.p0", database, table, partition)
	}
}

func TestTiDBPlacementPolicyIDParsingRejectsInvalidIDs(t *testing.T) {
	if _, err := parseTiDatabasePlacementPolicyID(""); err == nil {
		t.Fatal("expected empty database placement import ID to fail")
	}
	if _, _, err := parseTiTablePlacementPolicyID("app"); err == nil {
		t.Fatal("expected table placement import ID without table to fail")
	}
	if _, _, _, err := parseTiPartitionPlacementPolicyID("app.orders"); err == nil {
		t.Fatal("expected partition placement import ID without partition to fail")
	}
}

func TestNormalizeTiDBPlacementPolicyReadback(t *testing.T) {
	if got := normalizeTiDBPlacementPolicyReadback(""); got != "default" {
		t.Fatalf("normalizeTiDBPlacementPolicyReadback(\"\") = %q, want default", got)
	}
	if got := normalizeTiDBPlacementPolicyReadback("regional"); got != "regional" {
		t.Fatalf("normalizeTiDBPlacementPolicyReadback(\"regional\") = %q, want regional", got)
	}
}
