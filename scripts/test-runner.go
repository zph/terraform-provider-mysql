package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zph/terraform-provider-mysql/v3/internal/testmatrix"
)

type databaseType = testmatrix.Database

const (
	dbMySQL   = testmatrix.MySQL
	dbPercona = testmatrix.Percona
	dbMariaDB = testmatrix.MariaDB
	dbTiDB    = testmatrix.TiDB
)

const (
	cliDBMySQL   = "mysql"
	cliDBPercona = "percona"
	cliDBMariaDB = "mariadb"
	cliDBTiDB    = "tidb"
)

const (
	defaultTestPackage = "./mysql/..."
	defaultTestTimeout = "30m"
	defaultTestPattern = runAllTestPattern
	runAllTestPattern  = "."
)

const (
	envDockerDefaultPlatform = "DOCKER_DEFAULT_PLATFORM"
	envDockerHost            = "DOCKER_HOST"
	envDockerImage           = "DOCKER_IMAGE"
	envDockerPlatform        = "DOCKER_PLATFORM"
	envTestcontainersRyuk    = "TESTCONTAINERS_RYUK_DISABLED"
	envTerraformAcceptance   = "TF_ACC"
	envVerbose               = "VERBOSE"

	linuxAMD64Platform         = "linux/amd64"
	podmanSocketName           = "podman.sock"
	skipPodmanPercona57Message = "Podman amd64 emulation segfaults"
)

type testResult struct {
	image       string
	dbType      databaseType
	passed      bool
	skipped     bool
	skipReason  string
	logFile     string
	duration    time.Duration
	totalTests  int
	passedTests int
	failedTests int
}

type testJob struct {
	image       string
	dockerImage string
	dbType      databaseType
	testPattern string
	testPackage string
	timeout     string
	verbose     bool
	testNum     int
}

type testRunnerConfig struct {
	testPattern string
	testPackage string
	timeout     string
	image       string
	dbType      string
	version     string
	verbose     bool
}

type progressTracker struct {
	mu         sync.Mutex
	trackers   map[string]*versionProgress
	lastUpdate time.Time
}

type versionProgress struct {
	dbType        databaseType
	image         string
	totalTests    int
	passedTests   int
	failedTests   int
	skippedTests  int
	failedOutput  []string        // Store failure output lines
	failedTestSet map[string]bool // Track which tests have failed
	lastUpdate    time.Time
}

var (
	outputMutex sync.Mutex
	progress    = &progressTracker{
		trackers: make(map[string]*versionProgress),
	}
	totalJobsCount int
	isParallel     bool
)

type incrementalResultTable struct {
	printedHeader bool
}

func main() {
	cfg := withDefaultPattern(parseConfig())

	// Get parallelism from environment variable
	parallel := getParallelism()

	// Detect platform architecture
	isARM := isARMPlatform()

	fmt.Printf("Testcontainers Matrix Test Suite\n")
	fmt.Printf("Test pattern: %s | Parallelism: %d", cfg.testPattern, parallel)
	if cfg.verbose {
		fmt.Printf(" | Verbose")
	}
	if isARM {
		fmt.Printf(" | Platform: ARM (Apple Silicon)")
	}
	fmt.Printf("\n\n")

	jobs, skippedResults, err := buildJobs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	totalJobsCount = len(jobs) + len(skippedResults)

	resultTable := &incrementalResultTable{}
	resultTable.printHeader()
	for _, result := range skippedResults {
		resultTable.append(result)
	}

	// Run tests (sequentially or in parallel)
	var results []testResult
	if parallel > 1 {
		isParallel = true
		results = runTestsParallel(jobs, parallel, resultTable)
	} else {
		isParallel = false
		results = runTestsSequential(jobs, resultTable)
	}

	// Add skipped results to the results list
	results = append(results, skippedResults...)

	resultTable.close()

	// Print summary
	printSummary(results)

	// Exit with error code if any tests failed (skipped tests don't count as failures)
	for _, result := range results {
		if !result.passed && !result.skipped {
			os.Exit(1)
		}
	}
}

func parseConfig() testRunnerConfig {
	cfg := testRunnerConfig{}
	flag.StringVar(&cfg.testPattern, "pattern", "", "Go test -run pattern")
	flag.StringVar(&cfg.testPattern, "run", "", "Alias for --pattern")
	flag.StringVar(&cfg.testPackage, "package", defaultTestPackage, "Go package pattern to test")
	flag.StringVar(&cfg.timeout, "timeout", defaultTestTimeout, "Go test timeout")
	flag.StringVar(&cfg.image, "image", "", "Run one database image, e.g. mysql:8.0 or tidb:8.5.5")
	flag.StringVar(&cfg.dbType, "db", "", "Run one database type: mysql, percona, mariadb, tidb")
	flag.StringVar(&cfg.version, "version", "", "Database version for --db")
	flag.BoolVar(&cfg.verbose, "verbose", truthyEnv(envVerbose), "Stream underlying go test output")
	flag.Parse()

	if cfg.testPattern == "" && flag.NArg() > 0 {
		cfg.testPattern = flag.Arg(0)
	}
	return cfg
}

func buildJobs(cfg testRunnerConfig) ([]testJob, []testResult, error) {
	cfg = withDefaultPattern(cfg)

	if cfg.image != "" {
		job, err := jobFromImage(cfg.image, cfg, 1)
		if err != nil {
			return nil, nil, err
		}
		return jobsOrSkip(job)
	}

	if cfg.dbType != "" || cfg.version != "" {
		if cfg.dbType == "" || cfg.version == "" {
			return nil, nil, fmt.Errorf("--db and --version must be provided together")
		}
		image, err := imageForDBVersion(cfg.dbType, cfg.version)
		if err != nil {
			return nil, nil, err
		}
		job, err := jobFromImage(image, cfg, 1)
		if err != nil {
			return nil, nil, err
		}
		return jobsOrSkip(job)
	}

	jobs, skippedResults := matrixJobs(cfg)
	return jobs, skippedResults, nil
}

func withDefaultPattern(cfg testRunnerConfig) testRunnerConfig {
	if cfg.testPattern != "" {
		return cfg
	}
	if cfg.image == "" && cfg.dbType == "" && cfg.version == "" {
		cfg.testPattern = defaultTestPattern
	} else {
		cfg.testPattern = runAllTestPattern
	}
	return cfg
}

func matrixJobs(cfg testRunnerConfig) ([]testJob, []testResult) {
	var jobs []testJob
	var skippedResults []testResult
	testNum := 0

	for _, entry := range testmatrix.All() {
		testNum++
		job := newTestJob(entry.Database, entry.DisplayImage(), entry.DockerImage(), cfg, testNum)
		if skipped, ok := skipResult(job); ok {
			skippedResults = append(skippedResults, *skipped)
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, skippedResults
}

func jobsOrSkip(job testJob) ([]testJob, []testResult, error) {
	if skipped, ok := skipResult(job); ok {
		return nil, []testResult{*skipped}, nil
	}
	return []testJob{job}, nil, nil
}

func skipResult(job testJob) (*testResult, bool) {
	if shouldSkipForPodmanPercona57Emulation(job.dbType, job.dockerImage) {
		return &testResult{
			image:      job.image,
			dbType:     job.dbType,
			passed:     true,
			skipped:    true,
			skipReason: skipPodmanPercona57Message,
			duration:   0,
		}, true
	}
	return nil, false
}

func imageForDBVersion(dbType, version string) (string, error) {
	return testmatrix.ImageForDBVersion(dbType, version)
}

func jobFromImage(rawImage string, cfg testRunnerConfig, testNum int) (testJob, error) {
	image := normalizeImageAlias(rawImage)
	dbType, displayImage, err := inferDBTypeAndDisplayImage(image)
	if err != nil {
		return testJob{}, err
	}
	return newTestJob(dbType, displayImage, image, cfg, testNum), nil
}

func normalizeImageAlias(image string) string {
	return testmatrix.NormalizeImageAlias(image)
}

func inferDBTypeAndDisplayImage(image string) (databaseType, string, error) {
	return testmatrix.InferDatabaseAndDisplayImage(image)
}

func newTestJob(dbType databaseType, image, dockerImage string, cfg testRunnerConfig, testNum int) testJob {
	return testJob{
		image:       image,
		dockerImage: dockerImage,
		dbType:      dbType,
		testPattern: cfg.testPattern,
		testPackage: cfg.testPackage,
		timeout:     cfg.timeout,
		verbose:     cfg.verbose,
		testNum:     testNum,
	}
}

// isARMPlatform detects if we're running on ARM architecture (including Apple Silicon)
func isARMPlatform() bool {
	arch := runtime.GOARCH
	// Check for ARM architectures
	return arch == "arm64" || arch == "arm"
}

func needsAMD64Platform(dbType databaseType, image string) bool {
	return (dbType == dbMySQL && testmatrix.ImageIsInSeries(image, testmatrix.ImageMySQLPrefix, testmatrix.VersionMySQL57)) ||
		(dbType == dbPercona && testmatrix.ImageIsInSeries(image, testmatrix.ImagePerconaPrefix, testmatrix.VersionPercona57))
}

func shouldDisableRyukForPodman() bool {
	return os.Getenv(envTestcontainersRyuk) == "" && strings.Contains(os.Getenv(envDockerHost), podmanSocketName)
}

func shouldSkipForPodmanPercona57Emulation(dbType databaseType, image string) bool {
	return isARMPlatform() &&
		strings.Contains(os.Getenv(envDockerHost), podmanSocketName) &&
		dbType == dbPercona &&
		testmatrix.ImageIsInSeries(image, testmatrix.ImagePerconaPrefix, testmatrix.VersionPercona57)
}

func truthyEnv(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func getParallelism() int {
	parallelStr := os.Getenv("PARALLEL")
	if parallelStr == "" {
		return 1 // Default to sequential
	}

	parallel, err := strconv.Atoi(parallelStr)
	if err != nil || parallel < 1 {
		fmt.Fprintf(os.Stderr, "Warning: Invalid PARALLEL value '%s', using 1 (sequential)\n", parallelStr)
		return 1
	}

	// Cap parallelism at number of CPUs + 2 to avoid overwhelming the system
	maxParallel := runtime.NumCPU() + 2
	if parallel > maxParallel {
		fmt.Fprintf(os.Stderr, "Warning: PARALLEL=%d exceeds recommended max (%d), capping at %d\n", parallel, maxParallel, maxParallel)
		return maxParallel
	}

	return parallel
}

func runTestsSequential(jobs []testJob, resultTable *incrementalResultTable) []testResult {
	var results []testResult

	for _, job := range jobs {
		result := runTest(job)
		results = append(results, result)
		resultTable.append(result)
	}

	return results
}

func runTestsParallel(jobs []testJob, parallel int, resultTable *incrementalResultTable) []testResult {
	// Create job channel
	jobChan := make(chan testJob, len(jobs))
	resultChan := make(chan testResult, len(jobs))

	// Send all jobs to channel
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				result := runTest(job)
				resultChan <- result
			}
		}()
	}

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results as they come in
	var results []testResult
	for result := range resultChan {
		results = append(results, result)
		resultTable.append(result)
	}

	return results
}

func runTest(job testJob) testResult {
	key := fmt.Sprintf("%s-%s", job.dbType, job.image)

	// Initialize progress tracker
	progress.mu.Lock()
	progress.trackers[key] = &versionProgress{
		dbType:        job.dbType,
		image:         job.image,
		totalTests:    0,
		passedTests:   0,
		failedTests:   0,
		skippedTests:  0,
		failedOutput:  []string{},
		failedTestSet: make(map[string]bool),
		lastUpdate:    time.Now(),
	}
	progress.mu.Unlock()

	// Synchronize output to prevent interleaving
	outputMutex.Lock()
	if !isParallel && !job.verbose {
		// In sequential mode, show counter and start progress bar
		fmt.Fprintf(os.Stderr, "\n[%d/%d] ", job.testNum, getTotalJobs())
		progress.mu.Lock()
		tracker := progress.trackers[key]
		progress.mu.Unlock()
		renderProgress(tracker)
	} else if job.verbose {
		fmt.Printf("\n[%d/%d] %s %s\n", job.testNum, getTotalJobs(), job.dbType, extractVersion(job.image))
	}
	outputMutex.Unlock()

	// Sanitize image name for log file
	logFile := fmt.Sprintf("/tmp/testcontainers-%s-%s.log", job.dbType, sanitizeImageName(job.image))

	start := time.Now()

	// Build the go test command with JSON output
	cmd := exec.Command("go", "test",
		"-tags=testcontainers",
		"-json",
		job.testPackage,
		"-run", job.testPattern,
		"-count", "1",
		"-timeout", job.timeout,
	)

	// Set environment variables
	envVars := os.Environ()
	envVars = append(envVars, envDockerImage+"="+job.dockerImage)
	envVars = append(envVars, envTerraformAcceptance+"=1")
	if shouldDisableRyukForPodman() {
		envVars = append(envVars, envTestcontainersRyuk+"=true")
	}

	// Handle platform-specific issues for older MySQL/Percona versions on ARM64
	// MySQL 5.7 and Percona 5.7 don't have ARM64 builds.
	// Docker Desktop on Apple Silicon needs explicit platform specification
	if needsAMD64Platform(job.dbType, job.dockerImage) {
		envVars = append(envVars, envDockerDefaultPlatform+"="+linuxAMD64Platform)
		envVars = append(envVars, envDockerPlatform+"="+linuxAMD64Platform)
	}

	cmd.Env = envVars

	// Create log file
	logFileHandle, err := os.Create(logFile)
	if err != nil {
		outputMutex.Lock()
		fmt.Fprintf(os.Stderr, "Error creating log file %s: %v\n", logFile, err)
		outputMutex.Unlock()
		return testResult{
			image:   job.image,
			dbType:  job.dbType,
			passed:  false,
			logFile: logFile,
		}
	}
	defer logFileHandle.Close()

	// Create a pipe to capture output in real-time
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		outputMutex.Lock()
		fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
		outputMutex.Unlock()
		return testResult{
			image:   job.image,
			dbType:  job.dbType,
			passed:  false,
			logFile: logFile,
		}
	}

	// Set command output to pipe
	cmd.Stdout = pipeWriter
	cmd.Stderr = pipeWriter

	// Start parsing output in a goroutine
	done := make(chan bool)
	var lineBuffer strings.Builder
	go func() {
		defer pipeReader.Close()
		buf := make([]byte, 4096)
		for {
			n, readErr := pipeReader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				// Write to log file
				logFileHandle.WriteString(chunk)

				// Accumulate lines for JSON parsing
				lineBuffer.WriteString(chunk)
				lines := strings.Split(lineBuffer.String(), "\n")
				// Keep the last incomplete line in buffer
				lineBuffer.Reset()
				if len(lines) > 1 {
					lineBuffer.WriteString(lines[len(lines)-1])
					// Process complete lines
					for i := 0; i < len(lines)-1; i++ {
						parseTestOutput(lines[i], key, job.verbose)
					}
				}
			}
			if readErr != nil {
				// Process any remaining line in buffer
				if lineBuffer.Len() > 0 {
					parseTestOutput(lineBuffer.String(), key, job.verbose)
				}
				break
			}
		}
		done <- true
	}()

	// Run the command
	err = cmd.Run()

	// Close writer to signal EOF to reader
	pipeWriter.Close()

	// Wait for reader goroutine to finish
	<-done
	duration := time.Since(start)

	// Get final progress counts and failure output
	progress.mu.Lock()
	tracker := progress.trackers[key]
	var totalTests, passedTests, failedTests int
	var failedOutput []string
	if tracker != nil {
		totalTests = tracker.totalTests
		passedTests = tracker.passedTests
		failedTests = tracker.failedTests
		failedOutput = tracker.failedOutput

		// Render final progress state in sequential mode. Parallel mode reports
		// completed suites through the incremental results table.
		if tracker.totalTests > 0 && !isParallel && !job.verbose {
			outputMutex.Lock()
			// Clear the in-progress line
			fmt.Fprintf(os.Stderr, "\r\033[K")
			renderProgress(tracker)
			fmt.Fprintf(os.Stderr, "\n")
			outputMutex.Unlock()
		}
	}
	progress.mu.Unlock()

	outputMutex.Lock()
	passed := err == nil

	// Print failure output if there were failures
	if failedTests > 0 && len(failedOutput) > 0 {
		fmt.Println()
		for _, line := range failedOutput {
			fmt.Print(line)
		}
		fmt.Println()
	}

	outputMutex.Unlock()

	return testResult{
		image:       job.image,
		dbType:      job.dbType,
		passed:      passed,
		logFile:     logFile,
		duration:    duration,
		totalTests:  totalTests,
		passedTests: passedTests,
		failedTests: failedTests,
		// Note: skippedTests is tracked but not stored in testResult struct
		// It's displayed in the progress bar
	}
}

func getTotalJobs() int {
	return totalJobsCount
}

type testEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Elapsed float64   `json:"Elapsed"`
	Output  string    `json:"Output"`
}

func parseTestOutput(line string, key string, verbose bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	var event testEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Not valid JSON, skip
		return
	}

	if verbose && event.Output != "" {
		outputMutex.Lock()
		fmt.Print(event.Output)
		outputMutex.Unlock()
	}

	progress.mu.Lock()
	tracker := progress.trackers[key]
	if tracker == nil {
		progress.mu.Unlock()
		return
	}

	// Handle test-level events
	if event.Test != "" {
		switch event.Action {
		case "run":
			// Test started - increment total only once per test
			// Note: We count "run" events, but the actual completion is tracked via "pass"/"fail"/"skip"
			tracker.totalTests++
		case "pass":
			// Test passed - increment passed count
			tracker.passedTests++
		case "fail":
			// Test failed - increment failed count and mark test as failed
			tracker.failedTests++
			if event.Test != "" {
				tracker.failedTestSet[event.Test] = true
			}
		case "skip":
			// Test skipped - increment skipped count
			tracker.skippedTests++
		}
	}

	// Capture output from tests (especially failures)
	// The JSON format includes output events with the Test field set
	if event.Action == "output" && event.Output != "" && event.Test != "" {
		output := event.Output
		// Capture output from tests that have failed or look like failures
		// Output events come before the "fail" action, so we capture based on content
		// We'll also capture all output from tests that eventually fail
		isFailureOutput := strings.Contains(output, "FAIL:") ||
			strings.Contains(output, "--- FAIL:") ||
			strings.Contains(output, "Error:") ||
			strings.Contains(output, "panic:") ||
			(strings.Contains(output, "got:") && strings.Contains(output, "want:"))

		// Also capture if this test has already been marked as failed
		if isFailureOutput || tracker.failedTestSet[event.Test] {
			tracker.failedOutput = append(tracker.failedOutput, output)
		}
	}

	tracker.lastUpdate = time.Now()
	progress.mu.Unlock()

	// Update progress display when we have tests running
	if !verbose && tracker.totalTests > 0 && (tracker.passedTests > 0 || tracker.failedTests > 0) {
		outputMutex.Lock()
		if !isParallel {
			// In sequential mode, update progress on same line
			renderProgress(tracker)
		}
		// In parallel mode, don't update in real-time to avoid screen chaos
		outputMutex.Unlock()
	}
}

func renderProgress(tracker *versionProgress) {
	if tracker == nil {
		// Show empty bar initially
		bar := strings.Repeat("-", 30)
		fmt.Fprintf(os.Stderr, "\r\033[K[%s] 0/0", bar)
		return
	}

	version := extractVersion(tracker.image)
	if tracker.totalTests == 0 {
		// Show empty bar initially
		bar := strings.Repeat("-", 30)
		fmt.Fprintf(os.Stderr, "\r\033[K%-8s %-6s [%s] 0/0", tracker.dbType, version, bar)
		return
	}

	// Calculate percent based on completed tests (passed + failed), not including skipped
	completedTests := tracker.passedTests + tracker.failedTests
	var percent float64
	if tracker.totalTests > 0 {
		percent = float64(completedTests) / float64(tracker.totalTests)
	}
	barWidth := 30
	filled := int(percent * float64(barWidth))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)

	// Show passed/total, and include skipped/failed info if present
	// Pad database type to 8 chars and version to 6 chars for alignment
	status := fmt.Sprintf("%-8s %-6s [%s] %d/%d", tracker.dbType, version, bar, tracker.passedTests, tracker.totalTests)
	if tracker.failedTests > 0 {
		status = fmt.Sprintf("%-8s %-6s [%s] %d passed, %d failed", tracker.dbType, version, bar, tracker.passedTests, tracker.failedTests)
	} else if tracker.skippedTests > 0 {
		// If all non-skipped tests passed, show skipped count
		status = fmt.Sprintf("%-8s %-6s [%s] %d passed, %d skipped", tracker.dbType, version, bar, tracker.passedTests, tracker.skippedTests)
	}
	fmt.Fprintf(os.Stderr, "\r\033[K%s", status)
}

func printProgressBar(dbType, image string, total, passed, failed int, final bool) {
	version := extractVersion(image)

	if final {
		// Final: print summary
		if failed == 0 {
			fmt.Printf("  %s %s %d/%d passed\n", dbType, version, passed, total)
		} else {
			fmt.Printf("  %s %s %d passed, %d failed\n", dbType, version, passed, failed)
		}
	} else {
		// For in-progress, we use the progress bar library which handles updates
		// This is called from updateProgressBar which manages the actual bar
		// Just print a simple status line
		if isParallel {
			fmt.Printf("  %s %s %d passed", dbType, version, passed)
		} else {
			fmt.Printf("\r\033[K  %s %s %d passed", dbType, version, passed)
		}
		if failed > 0 {
			fmt.Printf(", %d failed", failed)
		}
	}
}

func sanitizeImageName(image string) string {
	// Replace colons and slashes with underscores
	result := strings.ReplaceAll(image, ":", "_")
	result = strings.ReplaceAll(result, "/", "_")
	return result
}

var resultTableColumns = []struct {
	name  string
	width int
}{
	{name: "Database", width: 10},
	{name: "Version", width: 10},
	{name: "Status", width: 44},
	{name: "Duration", width: 10},
}

func (t *incrementalResultTable) printHeader() {
	outputMutex.Lock()
	defer outputMutex.Unlock()

	t.printHeaderLocked()
}

func (t *incrementalResultTable) printHeaderLocked() {
	if t.printedHeader {
		return
	}

	fmt.Println("Results:")
	printResultTableBorder()
	printResultTableRow([]string{"Database", "Version", "Status", "Duration"})
	printResultTableBorder()
	t.printedHeader = true
}

func (t *incrementalResultTable) append(result testResult) {
	outputMutex.Lock()
	defer outputMutex.Unlock()

	t.printHeaderLocked()
	printResultTableRow([]string{
		result.dbType.Label(),
		extractVersion(result.image),
		resultStatus(result),
		formatDuration(result.duration),
	})
}

func (t *incrementalResultTable) close() {
	outputMutex.Lock()
	defer outputMutex.Unlock()

	if t.printedHeader {
		printResultTableBorder()
	}
}

func printResultTableBorder() {
	fmt.Print("+")
	for _, col := range resultTableColumns {
		fmt.Print(strings.Repeat("-", col.width+2))
		fmt.Print("+")
	}
	fmt.Println()
}

func printResultTableRow(values []string) {
	fmt.Print("|")
	for i, col := range resultTableColumns {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		value = fitTableValue(value, col.width)
		fmt.Printf(" %-*s |", col.width, value)
	}
	fmt.Println()
}

func fitTableValue(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func resultStatus(result testResult) string {
	if result.skipped {
		return fmt.Sprintf("SKIP (%s)", result.skipReason)
	}

	status := "PASS"
	if !result.passed {
		status = "FAIL"
	}

	if result.totalTests == 0 {
		return status
	}

	if result.failedTests > 0 {
		return fmt.Sprintf("%s (%d/%d, %d failed)", status, result.passedTests, result.totalTests, result.failedTests)
	}

	return fmt.Sprintf("%s (%d/%d)", status, result.passedTests, result.totalTests)
}

func printSummary(results []testResult) {
	fmt.Println()

	passed := 0
	failed := 0
	skippedCount := 0

	for _, result := range results {
		if result.skipped {
			skippedCount++
		} else if !result.passed {
			failed++
		} else {
			passed++
		}
	}

	totalRun := passed + failed
	fmt.Printf("\nSummary: %d/%d passed", passed, totalRun)
	if skippedCount > 0 {
		fmt.Printf(", %d skipped", skippedCount)
	}
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	if skippedCount > 0 {
		fmt.Printf(" (%d total test suites)", len(results))
	}
	fmt.Println()

	if failed > 0 {
		fmt.Println("Logs available in /tmp/testcontainers-*.log")
	}
}

func extractVersion(image string) string {
	// Extract version from image string
	// Examples: "mysql:8.0" -> "8.0", "tidb:6.1.7" -> "6.1.7", "percona:5.7" -> "5.7"
	parts := strings.Split(image, ":")
	if len(parts) > 1 {
		return parts[1]
	}
	return image
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Nanoseconds())/1e6)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}
