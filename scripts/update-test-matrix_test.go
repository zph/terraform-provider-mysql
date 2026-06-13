package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/zph/terraform-provider-mysql/v3/internal/testmatrix"
)

func TestMariaDBCommunityEOLDate(t *testing.T) {
	originalCache := textCache
	textCache = map[string]string{
		mariadbMaintenancePolicySource: `
			<table>
				<tr><td>12.3</td><td>TBC</td><td>TBC</td><td>TBC</td><td>TBC</td></tr>
				<tr><td>11.8</td><td>4 Jun 2025</td><td>4 Jun 2028</td><td>22 Oct 2030</td><td>22 Oct 2033</td></tr>
			</table>
		`,
	}
	defer func() { textCache = originalCache }()

	eol, err := mariadbCommunityEOLDate(&http.Client{}, "11.8")
	if err != nil {
		t.Fatalf("expected MariaDB 11.8 EOL date: %v", err)
	}
	if eol != "2028-06-04" {
		t.Fatalf("expected MariaDB 11.8 EOL 2028-06-04, got %q", eol)
	}

	if _, err := mariadbCommunityEOLDate(&http.Client{}, "12.3"); err == nil {
		t.Fatal("expected MariaDB 12.3 TBC maintenance dates to be rejected")
	}
}

func TestRawMessageIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "missing", raw: nil, want: false},
		{name: "false bool", raw: json.RawMessage(`false`), want: false},
		{name: "true bool", raw: json.RawMessage(`true`), want: true},
		{name: "date string", raw: json.RawMessage(`"2023-07-18"`), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rawMessageIsTruthy(tt.raw); got != tt.want {
				t.Fatalf("rawMessageIsTruthy(%s) = %t, want %t", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMaxResultSeverity(t *testing.T) {
	results := []matrixCheckResult{
		{Entry: testmatrix.Entry{Database: testmatrix.MySQL, Cycle: "8.0", Version: "8.0"}},
		{Entry: testmatrix.Entry{Database: testmatrix.TiDB, Cycle: "8.5", Version: "8.5.5"}, PatchWarning: true},
	}

	if got := maxResultSeverity(results); got != severityYellow {
		t.Fatalf("maxResultSeverity without red = %v, want %v", got, severityYellow)
	}

	results = append(results, matrixCheckResult{
		Entry:     testmatrix.Entry{Database: testmatrix.MariaDB, Cycle: "11.8", Version: "-"},
		NewerLine: true,
	})

	if got := maxResultSeverity(results); got != severityRed {
		t.Fatalf("maxResultSeverity with red = %v, want %v", got, severityRed)
	}
}

func TestResultSummaryEscapesGitHubActionsMessage(t *testing.T) {
	message := githubActionsEscape("patch 100%\nsecond line\r")
	want := "patch 100%25%0Asecond line%0D"
	if message != want {
		t.Fatalf("githubActionsEscape() = %q, want %q", message, want)
	}
}
