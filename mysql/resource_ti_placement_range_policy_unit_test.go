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
