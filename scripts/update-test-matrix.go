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

	"github.com/zph/terraform-provider-mysql/v3/internal/testmatrix"
)

const (
	fileTestMatrix = "internal/testmatrix/matrix.go"

	dockerHubTagURL = "https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100"
	eolAPIURL       = "https://endoflife.date/api/%s.json"
)

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

func main() {
	write := flag.Bool("write", false, "rewrite known matrix versions in repo files")
	failOnWarning := flag.Bool("fail-on-warning", false, "exit non-zero when drift or EOL warnings are found")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	var replacements []versionReplacement
	hadWarning := false

	for _, entry := range testmatrix.All() {
		latest, err := latestPatchTag(client, entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN %s %s: latest patch lookup failed: %v\n", entry.Database, entry.Cycle, err)
			hadWarning = true
			continue
		}
		if latest != entry.Version {
			fmt.Printf("UPDATE %s %s: %s -> %s\n", entry.Database, entry.Cycle, entry.Version, latest)
			replacements = append(replacements, versionReplacement{Entry: entry, Old: entry.Version, New: latest})
			hadWarning = true
		} else {
			fmt.Printf("OK     %s %s: %s\n", entry.Database, entry.Cycle, entry.Version)
		}

		if entry.EOLProduct == "" {
			fmt.Printf("WARN   %s %s: no EOL API configured; check vendor release policy manually\n", entry.Database, entry.Cycle)
			hadWarning = true
			continue
		}

		eol, err := eolForCycle(client, entry.EOLProduct, entry.EOLCycle)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN %s %s: EOL lookup failed: %v\n", entry.Database, entry.Cycle, err)
			hadWarning = true
			continue
		}
		if eol != "" && eol != "false" && eol != "true" {
			if eolDate, err := time.Parse(time.DateOnly, eol); err == nil && time.Now().After(eolDate) {
				proxy := ""
				if entry.EOLProxyFor != "" {
					proxy = fmt.Sprintf(" using %s as proxy", entry.EOLProduct)
				}
				fmt.Printf("WARN   %s %s: EOL on %s%s\n", entry.Database, entry.Cycle, eol, proxy)
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

func latestPatchTag(client *http.Client, entry testmatrix.Entry) (string, error) {
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
