package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type matrixDatabase string

const (
	matrixMySQL   matrixDatabase = "mysql"
	matrixPercona matrixDatabase = "percona"
	matrixMariaDB matrixDatabase = "mariadb"
	matrixTiDB    matrixDatabase = "tidb"
)

const (
	fileTestRunner     = "scripts/test-runner.go"
	fileMatrixUpdater  = "scripts/update-test-matrix.go"
	fileGitHubWorkflow = ".github/workflows/main.yml"

	dockerHubTagURL = "https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100"
	eolAPIURL       = "https://endoflife.date/api/%s.json"
)

type matrixEntry struct {
	Name        matrixDatabase
	Cycle       string
	Current     string
	DockerRepo  string
	TagPrefix   string
	BuildSuffix bool
	EOLProduct  string
	EOLCycle    string
	EOLProxyFor string
}

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
}

var matrixEntries = []matrixEntry{
	{Name: matrixMySQL, Cycle: "5.7", Current: "5.7", DockerRepo: "library/mysql", EOLProduct: "mysql", EOLCycle: "5.7"},
	{Name: matrixMySQL, Cycle: "8.0", Current: "8.0", DockerRepo: "library/mysql", EOLProduct: "mysql", EOLCycle: "8.0"},
	{Name: matrixPercona, Cycle: "5.7", Current: "5.7", DockerRepo: "library/percona", BuildSuffix: true, EOLProduct: "mysql", EOLCycle: "5.7", EOLProxyFor: "Percona Server"},
	{Name: matrixPercona, Cycle: "8.0", Current: "8.0", DockerRepo: "percona/percona-server", BuildSuffix: true, EOLProduct: "mysql", EOLCycle: "8.0", EOLProxyFor: "Percona Server"},
	{Name: matrixMariaDB, Cycle: "10.3", Current: "10.3", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "10.3"},
	{Name: matrixMariaDB, Cycle: "10.8", Current: "10.8", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "10.8"},
	{Name: matrixMariaDB, Cycle: "10.10", Current: "10.10", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "10.10"},
	{Name: matrixTiDB, Cycle: "6.1", Current: "6.1.7", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Name: matrixTiDB, Cycle: "6.5", Current: "6.5.12", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Name: matrixTiDB, Cycle: "7.1", Current: "7.1.6", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Name: matrixTiDB, Cycle: "7.5", Current: "7.5.7", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Name: matrixTiDB, Cycle: "8.1", Current: "8.1.2", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Name: matrixTiDB, Cycle: "8.5", Current: "8.5.5", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
}

func main() {
	write := flag.Bool("write", false, "rewrite known matrix versions in repo files")
	failOnWarning := flag.Bool("fail-on-warning", false, "exit non-zero when drift or EOL warnings are found")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	var replacements []versionReplacement
	hadWarning := false

	for _, entry := range matrixEntries {
		latest, err := latestPatchTag(client, entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN %s %s: latest patch lookup failed: %v\n", entry.Name, entry.Cycle, err)
			hadWarning = true
			continue
		}
		if latest != entry.Current {
			fmt.Printf("UPDATE %s %s: %s -> %s\n", entry.Name, entry.Cycle, entry.Current, latest)
			replacements = append(replacements, versionReplacement{Entry: entry, Old: entry.Current, New: latest})
			hadWarning = true
		} else {
			fmt.Printf("OK     %s %s: %s\n", entry.Name, entry.Cycle, entry.Current)
		}

		if entry.EOLProduct == "" {
			fmt.Printf("WARN   %s %s: no EOL API configured; check vendor release policy manually\n", entry.Name, entry.Cycle)
			hadWarning = true
			continue
		}

		eol, err := eolForCycle(client, entry.EOLProduct, entry.EOLCycle)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN %s %s: EOL lookup failed: %v\n", entry.Name, entry.Cycle, err)
			hadWarning = true
			continue
		}
		if eol != "" && eol != "false" && eol != "true" {
			if eolDate, err := time.Parse(time.DateOnly, eol); err == nil && time.Now().After(eolDate) {
				proxy := ""
				if entry.EOLProxyFor != "" {
					proxy = fmt.Sprintf(" using %s as proxy", entry.EOLProduct)
				}
				fmt.Printf("WARN   %s %s: EOL on %s%s\n", entry.Name, entry.Cycle, eol, proxy)
				hadWarning = true
			}
		}
	}

	if *write && len(replacements) > 0 {
		if err := applyReplacements(replacements); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR writing matrix updates: %v\n", err)
			os.Exit(1)
		}
	}

	if hadWarning && *failOnWarning {
		os.Exit(2)
	}
}

func latestPatchTag(client *http.Client, entry matrixEntry) (string, error) {
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

func eolForCycle(client *http.Client, product, cycle string) (string, error) {
	var cycles []eolCycle
	if err := getJSON(client, fmt.Sprintf(eolAPIURL, product), &cycles); err != nil {
		return "", err
	}
	for _, c := range cycles {
		if c.Cycle == cycle {
			var asString string
			if err := json.Unmarshal(c.EOL, &asString); err == nil {
				return asString, nil
			}
			var asBool bool
			if err := json.Unmarshal(c.EOL, &asBool); err == nil {
				return fmt.Sprintf("%t", asBool), nil
			}
			return "", nil
		}
	}
	return "", fmt.Errorf("cycle %s not found for %s", cycle, product)
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

type versionReplacement struct {
	Entry matrixEntry
	Old   string
	New   string
}

func applyReplacements(replacements []versionReplacement) error {
	if err := rewriteFile(fileTestRunner, func(content string) (string, error) {
		for _, replacement := range replacements {
			var err error
			content, err = replaceTestRunnerVersion(content, replacement)
			if err != nil {
				return "", err
			}
		}
		return content, nil
	}); err != nil {
		return err
	}

	if err := rewriteFile(fileMatrixUpdater, func(content string) (string, error) {
		for _, replacement := range replacements {
			var err error
			content, err = replaceUpdaterCurrentVersion(content, replacement)
			if err != nil {
				return "", err
			}
		}
		return content, nil
	}); err != nil {
		return err
	}

	if err := rewriteFile(fileGitHubWorkflow, func(content string) (string, error) {
		for _, replacement := range replacements {
			var err error
			content, err = replaceWorkflowVersion(content, replacement)
			if err != nil {
				return "", err
			}
		}
		return syncTiDBWorkflowComment(content), nil
	}); err != nil {
		return err
	}

	return nil
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

func replaceTestRunnerVersion(content string, replacement versionReplacement) (string, error) {
	return replaceOnce(
		fileTestRunner,
		content,
		replacement.Entry.testRunnerToken(replacement.Old),
		replacement.Entry.testRunnerToken(replacement.New),
	)
}

func replaceUpdaterCurrentVersion(content string, replacement versionReplacement) (string, error) {
	oldToken := fmt.Sprintf("Name: %s, Cycle: %q, Current: %q", replacement.Entry.constName(), replacement.Entry.Cycle, replacement.Old)
	newToken := fmt.Sprintf("Name: %s, Cycle: %q, Current: %q", replacement.Entry.constName(), replacement.Entry.Cycle, replacement.New)
	return replaceOnce(fileMatrixUpdater, content, oldToken, newToken)
}

func replaceWorkflowVersion(content string, replacement versionReplacement) (string, error) {
	oldBlock := fmt.Sprintf("db_version: %q\n          make_target: %q", replacement.Old, replacement.Entry.makeTarget(replacement.Old))
	newBlock := fmt.Sprintf("db_version: %q\n          make_target: %q", replacement.New, replacement.Entry.makeTarget(replacement.New))

	next, err := replaceOnce(fileGitHubWorkflow, content, oldBlock, newBlock)
	if err != nil {
		return "", err
	}

	if replacement.Entry.Name == matrixTiDB {
		next, err = replaceTiDBVersionsEnv(next, replacement.Old, replacement.New)
		if err != nil {
			return "", err
		}
	}
	return next, nil
}

func replaceOnce(file, content, oldToken, newToken string) (string, error) {
	next := strings.Replace(content, oldToken, newToken, 1)
	if next == content {
		return "", fmt.Errorf("could not find %q in %s", oldToken, file)
	}
	return next, nil
}

func replaceTiDBVersionsEnv(content, oldVersion, newVersion string) (string, error) {
	re := regexp.MustCompile(`TIDB_VERSIONS: "([^"]*)"`)
	matches := re.FindStringSubmatchIndex(content)
	if matches == nil {
		return "", fmt.Errorf("could not find TIDB_VERSIONS in %s", fileGitHubWorkflow)
	}

	versions := strings.Fields(content[matches[2]:matches[3]])
	replaced := false
	for i, version := range versions {
		if version == oldVersion {
			versions[i] = newVersion
			replaced = true
		}
	}
	if !replaced {
		return "", fmt.Errorf("could not find TiDB version %q in TIDB_VERSIONS", oldVersion)
	}

	newLine := `TIDB_VERSIONS: "` + strings.Join(versions, " ") + `"`
	return content[:matches[0]] + newLine + content[matches[1]:], nil
}

func syncTiDBWorkflowComment(content string) string {
	envRe := regexp.MustCompile(`TIDB_VERSIONS: "([^"]*)"`)
	env := envRe.FindStringSubmatch(content)
	if len(env) != 2 {
		return content
	}

	commentRe := regexp.MustCompile(`# TiDB versions - must match env\.TIDB_VERSIONS: .*`)
	return commentRe.ReplaceAllString(content, "# TiDB versions - must match env.TIDB_VERSIONS: "+env[1])
}

func (entry matrixEntry) testRunnerToken(version string) string {
	if entry.Name == matrixTiDB {
		return strconv.Quote(version)
	}
	return strconv.Quote(entry.dockerImage(version))
}

func (entry matrixEntry) dockerImage(version string) string {
	switch entry.Name {
	case matrixMySQL:
		return "mysql:" + version
	case matrixPercona:
		if strings.HasPrefix(version, "8.0") {
			return "percona/percona-server:" + version
		}
		return "percona:" + version
	case matrixMariaDB:
		return "mariadb:" + version
	default:
		return string(entry.Name) + ":" + version
	}
}

func (entry matrixEntry) makeTarget(version string) string {
	return fmt.Sprintf("test-%s-%s", entry.Name, version)
}

func (entry matrixEntry) constName() string {
	switch entry.Name {
	case matrixMySQL:
		return "matrixMySQL"
	case matrixPercona:
		return "matrixPercona"
	case matrixMariaDB:
		return "matrixMariaDB"
	case matrixTiDB:
		return "matrixTiDB"
	default:
		return string(entry.Name)
	}
}

func compareVersions(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
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
