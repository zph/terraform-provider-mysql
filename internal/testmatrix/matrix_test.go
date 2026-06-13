package testmatrix

import "testing"

func TestActionsMatrixUsesSingleEntrySource(t *testing.T) {
	matrix := ActionsMatrix()
	entries := All()
	if len(matrix.Include) != len(entries) {
		t.Fatalf("got %d matrix rows, want %d", len(matrix.Include), len(entries))
	}

	seenTargets := map[string]bool{}
	for i, row := range matrix.Include {
		entry := entries[i]
		if row.DBType != entry.Database.CLIName() {
			t.Fatalf("row %d db_type = %q, want %q", i, row.DBType, entry.Database.CLIName())
		}
		if row.DBVersion != entry.Version {
			t.Fatalf("row %d db_version = %q, want %q", i, row.DBVersion, entry.Version)
		}
		key := row.DBType + ":" + row.DBVersion
		if seenTargets[key] {
			t.Fatalf("duplicate matrix row %q", key)
		}
		seenTargets[key] = true
	}
}

func TestEntryImages(t *testing.T) {
	tests := []struct {
		name        string
		entry       Entry
		wantDisplay string
		wantDocker  string
	}{
		{
			name:        "mysql",
			entry:       Entry{Database: MySQL, Version: "8.0"},
			wantDisplay: "mysql:8.0",
			wantDocker:  "mysql:8.0",
		},
		{
			name:        "percona 8 uses native image",
			entry:       Entry{Database: Percona, Version: "8.0.46-37"},
			wantDisplay: "percona/percona-server:8.0.46-37",
			wantDocker:  "percona/percona-server:8.0.46-37",
		},
		{
			name:        "percona 8.4 uses native image",
			entry:       Entry{Database: Percona, Version: "8.4.8-8"},
			wantDisplay: "percona/percona-server:8.4.8-8",
			wantDocker:  "percona/percona-server:8.4.8-8",
		},
		{
			name:        "tidb displays version",
			entry:       Entry{Database: TiDB, Version: "8.5.5"},
			wantDisplay: "8.5.5",
			wantDocker:  "tidb:8.5.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.DisplayImage(); got != tt.wantDisplay {
				t.Fatalf("DisplayImage() = %q, want %q", got, tt.wantDisplay)
			}
			if got := tt.entry.DockerImage(); got != tt.wantDocker {
				t.Fatalf("DockerImage() = %q, want %q", got, tt.wantDocker)
			}
		})
	}
}
