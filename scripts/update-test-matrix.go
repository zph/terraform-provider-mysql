package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/zph/terraform-provider-mysql/v3/internal/testmatrix"
)

const (
	fileTestMatrix = "internal/testmatrix/matrix.go"

	dockerHubTagURL                 = "https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100"
	eolAPIURL                       = "https://endoflife.date/api/%s.json"
	mariadbMaintenancePolicySource  = "https://mariadb.org/about/"
	tidbSelfManagedReleaseNotesURL  = "https://docs.pingcap.com/releases/tidb-self-managed.md"
	tidbReleaseSupportPolicySource  = "https://www.pingcap.com/tidb-release-support-policy/"
	tidbReleaseSupportPolicyProduct = "TiDB Community Edition"
)

var textCache = map[string]string{}

type dockerHubTagsResponse struct {
	Next    string `json:"next"`
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

type eolCycle struct {
	Cycle  string          `json:"cycle"`
	EOL    json.RawMessage `json:"eol"`
	Latest string          `json:"latest"`
	LTS    json.RawMessage `json:"lts"`
}

type matrixCheckResult struct {
	Entry        testmatrix.Entry
	Latest       string
	EOL          string
	Replacement  *versionReplacement
	PatchWarning bool
	EOLWarning   bool
	NewerLine    bool
}

type resultSeverity int

const (
	severityGreen resultSeverity = iota
	severityYellow
	severityRed
)

func main() {
	write := flag.Bool("write", false, "rewrite known matrix versions in repo files")
	failOnWarning := flag.Bool("fail-on-warning", false, "exit non-zero when drift or EOL warnings are found")
	failOnRed := flag.Bool("fail-on-red", false, "exit non-zero when any row has a red status")
	githubActions := flag.Bool("github-actions", false, "emit GitHub Actions warning/error annotations")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	var replacements []versionReplacement
	hadWarning := false
	results := make([]matrixCheckResult, 0, len(testmatrix.All()))
	stopSpinner := startSpinner("Checking matrix versions")

	for _, entry := range testmatrix.All() {
		result := checkMatrixEntry(client, entry)
		results = append(results, result)
		if result.HasWarning() {
			hadWarning = true
		}
		if result.Replacement != nil {
			replacements = append(replacements, *result.Replacement)
		}
	}
	for _, result := range newerLineResults(client, testmatrix.All()) {
		results = append(results, result)
		if result.HasWarning() {
			hadWarning = true
		}
	}

	stopSpinner()
	printResultsTable(results)
	if *githubActions {
		reportGitHubActionsStatus(results)
	}

	if *write && len(replacements) > 0 {
		if err := applyReplacements(replacements); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR writing matrix updates: %v\n", err)
			os.Exit(1)
		}
	}

	if *failOnRed && maxResultSeverity(results) == severityRed {
		os.Exit(2)
	}

	if hadWarning && *failOnWarning {
		os.Exit(2)
	}
}

func checkMatrixEntry(client *http.Client, entry testmatrix.Entry) matrixCheckResult {
	result := matrixCheckResult{
		Entry:  entry,
		Latest: "-",
		EOL:    "-",
	}

	warnPatch := func() {
		result.PatchWarning = true
	}
	warnEOL := func() {
		result.EOLWarning = true
	}

	latest, err := latestPatchVersion(client, entry)
	if err != nil {
		warnPatch()
	} else {
		result.Latest = latest
		if latest != entry.Version {
			warnPatch()
			result.Replacement = &versionReplacement{Entry: entry, Old: entry.Version, New: latest}
		}
	}

	if entry.Database != testmatrix.TiDB && entry.EOLProduct == "" {
		result.EOL = "manual"
		warnEOL()
		return result
	}

	eol, err := eolForEntry(client, entry)
	if err != nil {
		warnEOL()
		return result
	}
	result.EOL = displayEOL(eol)

	switch {
	case eol == "" || eol == "false":
	case eol == "true":
		warnEOL()
	default:
		eolDate, err := time.Parse(time.DateOnly, eol)
		if err != nil {
			warnEOL()
			break
		}
		if time.Now().After(eolDate) {
			warnEOL()
		}
	}

	return result
}

func (result matrixCheckResult) HasWarning() bool {
	return result.PatchWarning || result.EOLWarning || result.NewerLine
}

func displayEOL(eol string) string {
	switch eol {
	case "":
		return "-"
	case "false":
		return "active"
	default:
		return eol
	}
}

func printResultsTable(results []matrixCheckResult) {
	sortMatrixCheckResults(results)

	writer := table.NewWriter()
	writer.SetStyle(table.StyleLight)
	writer.AppendHeader(table.Row{"Status", "Database", "Cycle", "Current", "Latest", "EOL Date", "Notes (patch/EOL)"})
	for _, result := range results {
		writer.AppendRow(table.Row{
			totalStatus(result),
			result.Entry.Database.Label(),
			result.Entry.Cycle,
			result.Entry.Version,
			result.Latest,
			result.EOL,
			resultNotes(result),
		})
	}
	fmt.Println(writer.Render())
}

func sortMatrixCheckResults(results []matrixCheckResult) {
	sort.SliceStable(results, func(i, j int) bool {
		left := results[i]
		right := results[j]

		leftDB := left.Entry.Database.Label()
		rightDB := right.Entry.Database.Label()
		if leftDB != rightDB {
			return leftDB < rightDB
		}

		cycleCompare := compareSemanticVersions(left.Entry.Cycle, right.Entry.Cycle)
		if cycleCompare != 0 {
			return cycleCompare < 0
		}

		versionCompare := compareSemanticVersions(left.Entry.Version, right.Entry.Version)
		if versionCompare != 0 {
			return versionCompare < 0
		}

		return left.Latest < right.Latest
	})
}

func totalStatus(result matrixCheckResult) string {
	return resultSeverityFor(result).Circle()
}

func statusCircle(ok bool) string {
	if ok {
		return "🟢"
	}
	return "🟡"
}

func resultNotes(result matrixCheckResult) string {
	patch := statusCircle(!result.PatchWarning)
	if result.NewerLine {
		patch = "🔴"
	}
	return fmt.Sprintf("%s/%s", patch, statusCircle(!result.EOLWarning))
}

func resultSeverityFor(result matrixCheckResult) resultSeverity {
	if result.NewerLine {
		return severityRed
	}
	if result.HasWarning() {
		return severityYellow
	}
	return severityGreen
}

func maxResultSeverity(results []matrixCheckResult) resultSeverity {
	maxSeverity := severityGreen
	for _, result := range results {
		if severity := resultSeverityFor(result); severity > maxSeverity {
			maxSeverity = severity
		}
	}
	return maxSeverity
}

func (severity resultSeverity) Circle() string {
	switch severity {
	case severityRed:
		return "🔴"
	case severityYellow:
		return "🟡"
	default:
		return "🟢"
	}
}

func reportGitHubActionsStatus(results []matrixCheckResult) {
	switch maxResultSeverity(results) {
	case severityRed:
		fmt.Fprintf(os.Stderr, "::error title=Matrix version policy::%s\n", githubActionsEscape(resultSummary(results, severityRed)))
	case severityYellow:
		fmt.Fprintf(os.Stderr, "::warning title=Matrix version policy::%s\n", githubActionsEscape(resultSummary(results, severityYellow)))
	}
}

func resultSummary(results []matrixCheckResult, severity resultSeverity) string {
	var summaries []string
	for _, result := range results {
		if resultSeverityFor(result) != severity {
			continue
		}
		summaries = append(summaries, fmt.Sprintf("%s %s current=%s latest=%s eol=%s",
			result.Entry.Database.Label(),
			result.Entry.Cycle,
			result.Entry.Version,
			result.Latest,
			result.EOL,
		))
	}

	switch severity {
	case severityRed:
		return "Red matrix rows require action: " + strings.Join(summaries, "; ")
	case severityYellow:
		return "Yellow matrix rows should be reviewed: " + strings.Join(summaries, "; ")
	default:
		return "Matrix versions are current and supported"
	}
}

func githubActionsEscape(message string) string {
	message = strings.ReplaceAll(message, "%", "%25")
	message = strings.ReplaceAll(message, "\r", "%0D")
	message = strings.ReplaceAll(message, "\n", "%0A")
	return message
}

func startSpinner(message string) func() {
	if !stderrIsTerminal() {
		return func() {}
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		frames := []string{"-", "\\", "|", "/"}
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for i := 0; ; i++ {
			fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], message)

			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
			}
		}
	}()

	return func() {
		close(done)
		wg.Wait()
	}
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func latestPatchVersion(client *http.Client, entry testmatrix.Entry) (string, error) {
	if entry.Database == testmatrix.TiDB {
		return latestTiDBPatchVersion(client, entry)
	}
	return latestDockerHubPatchTag(client, entry)
}

func newerLineResults(client *http.Client, entries []testmatrix.Entry) []matrixCheckResult {
	latestEntriesByDB := maxCycleEntries(entries)
	results := []matrixCheckResult{}

	for _, db := range []testmatrix.Database{testmatrix.MySQL, testmatrix.Percona, testmatrix.MariaDB, testmatrix.TiDB} {
		entry, ok := latestEntriesByDB[db]
		if !ok {
			continue
		}

		versions, err := newerActiveVersions(client, entry)
		if err != nil {
			results = append(results, matrixCheckResult{
				Entry: testmatrix.Entry{
					Database: entry.Database,
					Cycle:    "newer",
					Version:  "lookup failed",
				},
				Latest:       "-",
				EOL:          "-",
				PatchWarning: true,
			})
			continue
		}

		cycles := sortedVersionCycles(versions)
		for _, cycle := range cycles {
			if compareVersions(cycle, entry.Cycle) <= 0 {
				continue
			}

			eol, active := matrixCandidateEOLForNewerCycle(client, entry, cycle)
			if !active {
				continue
			}

			results = append(results, matrixCheckResult{
				Entry: testmatrix.Entry{
					Database: entry.Database,
					Cycle:    cycle,
					Version:  "-",
				},
				Latest:    versions[cycle],
				EOL:       displayEOL(eol),
				NewerLine: true,
			})
		}
	}

	return results
}

func maxCycleEntries(entries []testmatrix.Entry) map[testmatrix.Database]testmatrix.Entry {
	latest := make(map[testmatrix.Database]testmatrix.Entry)
	for _, entry := range entries {
		current, ok := latest[entry.Database]
		if !ok || compareVersions(entry.Cycle, current.Cycle) > 0 {
			latest[entry.Database] = entry
		}
	}
	return latest
}

func newerActiveVersions(client *http.Client, entry testmatrix.Entry) (map[string]string, error) {
	if entry.Database == testmatrix.TiDB {
		return tidbReleasedVersionsByCycle(client)
	}
	return dockerHubVersionsByCycle(client, entry)
}

func sortedVersionCycles(versions map[string]string) []string {
	cycles := make([]string, 0, len(versions))
	for cycle := range versions {
		cycles = append(cycles, cycle)
	}
	sort.Slice(cycles, func(i, j int) bool {
		return compareVersions(cycles[i], cycles[j]) < 0
	})
	return cycles
}

func matrixCandidateEOLForNewerCycle(client *http.Client, entry testmatrix.Entry, cycle string) (string, bool) {
	eol, err := matrixCandidateEOL(client, entry, cycle)
	if err != nil {
		return "", false
	}
	return eol, eolIsActive(eol)
}

func matrixCandidateEOL(client *http.Client, entry testmatrix.Entry, cycle string) (string, error) {
	switch entry.Database {
	case testmatrix.MariaDB:
		return mariadbCommunityEOLDate(client, cycle)
	case testmatrix.MySQL, testmatrix.Percona:
		candidate, err := eolCycleIsLTS(client, entry.EOLProduct, cycle)
		if err != nil {
			return "", err
		}
		if !candidate {
			return "", fmt.Errorf("cycle %s is not an LTS cycle for %s", cycle, entry.EOLProduct)
		}
	}

	eolEntry := entry
	eolEntry.Cycle = cycle
	eolEntry.EOLCycle = cycle

	return eolForEntry(client, eolEntry)
}

func eolIsActive(eol string) bool {
	if eol == "" || eol == "false" {
		return true
	}
	if eol == "true" {
		return false
	}

	eolDate, err := time.Parse(time.DateOnly, eol)
	if err != nil {
		return false
	}
	return !time.Now().After(eolDate)
}

func latestTiDBPatchVersion(client *http.Client, entry testmatrix.Entry) (string, error) {
	body, err := getCachedText(client, tidbSelfManagedReleaseNotesURL)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(entry.Cycle) + `\.\d+\b`)
	rawMatches := re.FindAllString(body, -1)
	if len(rawMatches) == 0 {
		return "", fmt.Errorf("no TiDB release notes match cycle %s in %s", entry.Cycle, tidbSelfManagedReleaseNotesURL)
	}

	seen := make(map[string]struct{}, len(rawMatches))
	matches := make([]string, 0, len(rawMatches))
	for _, match := range rawMatches {
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		matches = append(matches, match)
	}

	sort.Slice(matches, func(i, j int) bool {
		return compareVersions(matches[i], matches[j]) > 0
	})
	return matches[0], nil
}

func tidbReleasedVersionsByCycle(client *http.Client) (map[string]string, error) {
	body, err := getCachedText(client, tidbSelfManagedReleaseNotesURL)
	if err != nil {
		return nil, err
	}

	versions := map[string]string{}
	re := regexp.MustCompile(`\b\d+\.\d+\.\d+(?:-[A-Za-z0-9.]+)?\b`)
	for _, match := range re.FindAllString(body, -1) {
		if strings.Contains(match, "-") {
			continue
		}
		cycle := versionCycle(match)
		if cycle == "" {
			continue
		}
		if current, ok := versions[cycle]; !ok || compareVersions(match, current) > 0 {
			versions[cycle] = match
		}
	}

	return versions, nil
}

func latestDockerHubPatchTag(client *http.Client, entry testmatrix.Entry) (string, error) {
	tagPattern := "^" + regexp.QuoteMeta(entry.TagPrefix+entry.Cycle) + `\.\d+`
	if entry.BuildSuffix {
		tagPattern += `(?:-\d+)?`
	}
	tagPattern += "$"
	re := regexp.MustCompile(tagPattern)
	var matches []string
	url := fmt.Sprintf(dockerHubTagURL, entry.DockerRepo)

	for url != "" {
		var payload dockerHubTagsResponse
		if err := getJSON(client, url, &payload); err != nil {
			return "", err
		}
		for _, tag := range payload.Results {
			if re.MatchString(tag.Name) {
				matches = append(matches, strings.TrimPrefix(tag.Name, entry.TagPrefix))
			}
		}
		url = payload.Next
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no tags match cycle %s in %s", entry.Cycle, entry.DockerRepo)
	}

	sort.Slice(matches, func(i, j int) bool {
		return compareVersions(matches[i], matches[j]) > 0
	})
	return matches[0], nil
}

func dockerHubVersionsByCycle(client *http.Client, entry testmatrix.Entry) (map[string]string, error) {
	versionPattern := `\d+\.\d+\.\d+`
	if entry.BuildSuffix {
		versionPattern += `(?:-\d+)?`
	}
	re := regexp.MustCompile("^" + regexp.QuoteMeta(entry.TagPrefix) + "(" + versionPattern + ")$")
	versions := map[string]string{}
	url := fmt.Sprintf(dockerHubTagURL, entry.DockerRepo)

	for url != "" {
		var payload dockerHubTagsResponse
		if err := getJSON(client, url, &payload); err != nil {
			return nil, err
		}
		for _, tag := range payload.Results {
			matches := re.FindStringSubmatch(tag.Name)
			if len(matches) != 2 {
				continue
			}

			version := strings.TrimPrefix(matches[1], entry.TagPrefix)
			cycle := versionCycle(version)
			if cycle == "" {
				continue
			}
			if current, ok := versions[cycle]; !ok || compareVersions(version, current) > 0 {
				versions[cycle] = version
			}
		}
		url = payload.Next
	}

	return versions, nil
}

func versionCycle(version string) string {
	fields := regexp.MustCompile(`\d+`).FindAllString(version, -1)
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + "." + fields[1]
}

func eolForEntry(client *http.Client, entry testmatrix.Entry) (string, error) {
	if entry.Database == testmatrix.TiDB {
		return tidbCommunityEOLDate(client, entry.Cycle)
	}
	return eolForCycle(client, entry.EOLProduct, entry.EOLCycle)
}

func tidbCommunityEOLDate(client *http.Client, cycle string) (string, error) {
	body, err := getCachedText(client, tidbReleaseSupportPolicySource)
	if err != nil {
		return "", err
	}

	text := htmlText(body)
	re := regexp.MustCompile(`v\s*` + cycleRegex(cycle) + `\s+Community\s+Edition\s+\d{1,2}/\d{1,2}/\d{4}\s+\d{1,2}/\d{1,2}/\d{4}\s+(\d{1,2}/\d{1,2}/\d{4})`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return "", fmt.Errorf("no %s EOL date configured for cycle %s from %s", tidbReleaseSupportPolicyProduct, cycle, tidbReleaseSupportPolicySource)
	}

	eolDate, err := time.Parse("1/2/2006", matches[1])
	if err != nil {
		return "", fmt.Errorf("invalid %s EOL date %q for cycle %s from %s: %w", tidbReleaseSupportPolicyProduct, matches[1], cycle, tidbReleaseSupportPolicySource, err)
	}
	return eolDate.Format(time.DateOnly), nil
}

func mariadbCommunityEOLDate(client *http.Client, cycle string) (string, error) {
	body, err := getCachedText(client, mariadbMaintenancePolicySource)
	if err != nil {
		return "", err
	}

	text := htmlText(body)
	datePattern := `(?:\d{1,2}\s+[A-Za-z]{3}\s+\d{4}|TBC)`
	re := regexp.MustCompile(`\b` + cycleRegex(cycle) + `\s+` + datePattern + `\s+(` + datePattern + `)\s+` + datePattern + `\s+` + datePattern + `\b`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return "", fmt.Errorf("no MariaDB Community EOL date configured for cycle %s from %s", cycle, mariadbMaintenancePolicySource)
	}
	if matches[1] == "TBC" {
		return "", fmt.Errorf("MariaDB Community EOL date is TBC for cycle %s from %s", cycle, mariadbMaintenancePolicySource)
	}

	eolDate, err := time.Parse("2 Jan 2006", matches[1])
	if err != nil {
		return "", fmt.Errorf("invalid MariaDB Community EOL date %q for cycle %s from %s: %w", matches[1], cycle, mariadbMaintenancePolicySource, err)
	}
	return eolDate.Format(time.DateOnly), nil
}

func cycleRegex(cycle string) string {
	parts := strings.Split(cycle, ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, regexp.QuoteMeta(part))
	}
	return strings.Join(quoted, `\s*\.\s*`)
}

func eolForCycle(client *http.Client, product, cycle string) (string, error) {
	c, err := eolCycleFor(client, product, cycle)
	if err != nil {
		return "", err
	}
	return rawStringOrBool(c.EOL), nil
}

func eolCycleIsLTS(client *http.Client, product, cycle string) (bool, error) {
	c, err := eolCycleFor(client, product, cycle)
	if err != nil {
		return false, err
	}
	return rawMessageIsTruthy(c.LTS), nil
}

func eolCycleFor(client *http.Client, product, cycle string) (eolCycle, error) {
	var cycles []eolCycle
	if err := getJSON(client, fmt.Sprintf(eolAPIURL, product), &cycles); err != nil {
		return eolCycle{}, err
	}
	for _, c := range cycles {
		if c.Cycle == cycle {
			return c, nil
		}
	}
	return eolCycle{}, fmt.Errorf("cycle %s not found for %s", cycle, product)
}

func rawStringOrBool(raw json.RawMessage) string {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asBool bool
	if err := json.Unmarshal(raw, &asBool); err == nil {
		return fmt.Sprintf("%t", asBool)
	}
	return ""
}

func rawMessageIsTruthy(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	switch rawStringOrBool(raw) {
	case "", "false":
		return false
	default:
		return true
	}
}

func getJSON(client *http.Client, url string, out interface{}) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func getCachedText(client *http.Client, url string) (string, error) {
	if cached, ok := textCache[url]; ok {
		return cached, nil
	}
	body, err := getText(client, url)
	if err != nil {
		return "", err
	}
	textCache[url] = body
	return body, nil
}

func getText(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s returned %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func htmlText(body string) string {
	withoutTags := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(body, " ")
	unescaped := html.UnescapeString(withoutTags)
	return strings.Join(strings.Fields(unescaped), " ")
}

type versionReplacement struct {
	Entry testmatrix.Entry
	Old   string
	New   string
}

func applyReplacements(replacements []versionReplacement) error {
	return rewriteFile(fileTestMatrix, func(content string) (string, error) {
		for _, replacement := range replacements {
			var err error
			content, err = replaceMatrixVersion(content, replacement)
			if err != nil {
				return "", err
			}
		}
		return content, nil
	})
}

func rewriteFile(file string, mutate func(string) (string, error)) error {
	contentBytes, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	updated, err := mutate(content)
	if err != nil {
		return err
	}
	if updated == content {
		return nil
	}
	return os.WriteFile(file, []byte(updated), 0o644)
}

func replaceMatrixVersion(content string, replacement versionReplacement) (string, error) {
	oldToken := fmt.Sprintf("Database: %s, Cycle: %q, Version: %q", replacement.Entry.Database.ConstName(), replacement.Entry.Cycle, replacement.Old)
	newToken := fmt.Sprintf("Database: %s, Cycle: %q, Version: %q", replacement.Entry.Database.ConstName(), replacement.Entry.Cycle, replacement.New)
	return replaceOnce(fileTestMatrix, content, oldToken, newToken)
}

func replaceOnce(file, content, oldToken, newToken string) (string, error) {
	next := strings.Replace(content, oldToken, newToken, 1)
	if next == content {
		return "", fmt.Errorf("could not find %q in %s", oldToken, file)
	}
	return next, nil
}

func compareVersions(a, b string) int {
	return compareSemanticVersions(a, b)
}

func compareSemanticVersions(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	if len(ap) == 0 && len(bp) == 0 {
		return strings.Compare(a, b)
	}
	if len(ap) == 0 {
		return 1
	}
	if len(bp) == 0 {
		return -1
	}
	for i := 0; i < len(ap) || i < len(bp); i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func versionParts(version string) []int {
	fields := regexp.MustCompile(`\d+`).FindAllString(version, -1)
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, _ := strconv.Atoi(field)
		parts = append(parts, n)
	}
	return parts
}
