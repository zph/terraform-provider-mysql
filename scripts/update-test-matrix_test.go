package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestEOLCriticalSeverityAndNotes(t *testing.T) {
	result := matrixCheckResult{
		Entry:       testmatrix.Entry{Database: testmatrix.MySQL, Cycle: "5.7", Version: "5.7.44"},
		EOLWarning:  true,
		EOLCritical: true,
	}

	if got := resultSeverityFor(result); got != severityRed {
		t.Fatalf("resultSeverityFor() = %v, want %v", got, severityRed)
	}
	if got := resultNotes(result); got != "🟢/🔴" {
		t.Fatalf("resultNotes() = %q, want %q", got, "🟢/🔴")
	}
}

func TestEOLIsOlderThanPolicy(t *testing.T) {
	eolDate := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "before two years", now: time.Date(2026, 6, 12, 23, 59, 59, 0, time.UTC), want: false},
		{name: "exactly two years", now: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), want: false},
		{name: "after two years", now: time.Date(2026, 6, 13, 0, 0, 1, 0, time.UTC), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eolIsOlderThanPolicy(eolDate, tt.now); got != tt.want {
				t.Fatalf("eolIsOlderThanPolicy() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestResultSummaryEscapesGitHubActionsMessage(t *testing.T) {
	message := githubActionsEscape("patch 100%\nsecond line\r")
	want := "patch 100%25%0Asecond line%0D"
	if message != want {
		t.Fatalf("githubActionsEscape() = %q, want %q", message, want)
	}
}

func TestGitHubActionsPropertyEscape(t *testing.T) {
	message := githubActionsPropertyEscape("matrix: mysql, 100%\n")
	want := "matrix%3A mysql%2C 100%25%0A"
	if message != want {
		t.Fatalf("githubActionsPropertyEscape() = %q, want %q", message, want)
	}
}

func TestGitHubStepSummaryIncludesNonGreenRows(t *testing.T) {
	results := []matrixCheckResult{
		{Entry: testmatrix.Entry{Database: testmatrix.MySQL, Cycle: "8.0", Version: "8.0"}, Latest: "8.0", EOL: "2032-04-30"},
		{Entry: testmatrix.Entry{Database: testmatrix.TiDB, Cycle: "8.5", Version: "8.5.5"}, Latest: "8.5.6", EOL: "2028-12-19", PatchWarning: true},
		{Entry: testmatrix.Entry{Database: testmatrix.MariaDB, Cycle: "11.8", Version: "-"}, Latest: "11.8.8", EOL: "2028-06-04", NewerLine: true},
	}

	summary := githubStepSummary(results, severityRed)
	mustContain(t, summary, "## Matrix Version Policy")
	mustContain(t, summary, "**Failing:**")
	mustContain(t, summary, "| 🔴 | MariaDB | 11.8 | - | 11.8.8 | 2028-06-04 | 🔴/🟢 |")
	mustContain(t, summary, "| 🟡 | TiDB | 8.5 | 8.5.5 | 8.5.6 | 2028-12-19 | 🟡/🟢 |")
	mustNotContain(t, summary, "| 🟢 | MySQL | 8.0 |")
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q to contain %q", haystack, needle)
	}
}

func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected %q not to contain %q", haystack, needle)
	}
}
