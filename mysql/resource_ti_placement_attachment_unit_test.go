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

func TestTiDBPlacementPolicyIDParsingEscapedSeparators(t *testing.T) {
	database, table, err := parseTiTablePlacementPolicyID(`app\.prod.orders\.archive`)
	if err != nil {
		t.Fatalf("parseTiTablePlacementPolicyID returned error: %s", err)
	}
	if database != "app.prod" || table != "orders.archive" {
		t.Fatalf("table ID parsed as %q.%q, want app.prod.orders.archive", database, table)
	}

	database, table, partition, err := parseTiPartitionPlacementPolicyID(`app\.prod.orders\\archive.p\.0`)
	if err != nil {
		t.Fatalf("parseTiPartitionPlacementPolicyID returned error: %s", err)
	}
	if database != "app.prod" || table != `orders\archive` || partition != "p.0" {
		t.Fatalf("partition ID parsed as %q.%q.%q, want app.prod.orders\\archive.p.0", database, table, partition)
	}
}

func TestTiDBPlacementPolicyIDFormattingEscapedSeparators(t *testing.T) {
	if got := formatTiTablePlacementPolicyID("app.prod", "orders.archive"); got != `app\.prod.orders\.archive` {
		t.Fatalf("formatTiTablePlacementPolicyID() = %q, want app\\.prod.orders\\.archive", got)
	}

	if got := formatTiPartitionPlacementPolicyID("app.prod", `orders\archive`, "p.0"); got != `app\.prod.orders\\archive.p\.0` {
		t.Fatalf("formatTiPartitionPlacementPolicyID() = %q, want app\\.prod.orders\\\\archive.p\\.0", got)
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
	if _, _, err := parseTiTablePlacementPolicyID(`app.orders\archive`); err == nil {
		t.Fatal("expected table placement import ID with invalid escape to fail")
	}
	if _, _, err := parseTiTablePlacementPolicyID(`app.orders\`); err == nil {
		t.Fatal("expected table placement import ID with trailing escape to fail")
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
