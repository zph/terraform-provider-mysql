package mysql

import "testing"

func TestTiDBPlacementRangeTarget(t *testing.T) {
	tests := map[string]string{
		"global": "RANGE TiDB_GLOBAL",
		"meta":   "RANGE TiDB_META",
	}

	for input, want := range tests {
		if got := tiDBPlacementRangeTarget(input); got != want {
			t.Fatalf("tiDBPlacementRangeTarget(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseTiPlacementRangePolicyID(t *testing.T) {
	rangeName, placementPolicy, err := parseTiPlacementRangePolicyID("global:regional")
	if err != nil {
		t.Fatalf("parseTiPlacementRangePolicyID returned error: %s", err)
	}
	if rangeName != "global" || placementPolicy != "regional" {
		t.Fatalf("range ID parsed as %q:%q, want global:regional", rangeName, placementPolicy)
	}
}

func TestParseTiPlacementRangePolicyIDRejectsInvalidIDs(t *testing.T) {
	if _, _, err := parseTiPlacementRangePolicyID("global"); err == nil {
		t.Fatal("expected range placement import ID without policy to fail")
	}
	if _, _, err := parseTiPlacementRangePolicyID("table:regional"); err == nil {
		t.Fatal("expected unsupported range name to fail")
	}
}
