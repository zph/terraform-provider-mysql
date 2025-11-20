package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
)

var (
	// MySQL versions to test
	mysqlVersions = []string{
		"mysql:5.6",
		"mysql:5.7",
		"mysql:8.0",
	}

	// Percona versions to test
	perconaVersions = []string{
		"percona:5.7",
		"percona:8.0",
	}

	// MariaDB versions to test
	mariadbVersions = []string{
		"mariadb:10.3",
		"mariadb:10.8",
		"mariadb:10.10",
	}

	// TiDB versions to test (version numbers only, not full image names)
	tidbVersions = []string{
		"6.1.7",
		"6.5.12",
		"7.1.6",
		"7.5.7",
		"8.1.2",
		"8.5.3",
	}
)

type testResult struct {
	image       string
	dbType      string
	passed      bool
	logFile     string
	duration    time.Duration
	totalTests  int
	passedTests int
	failedTests int
}

type testJob struct {
	image       string
	dbType      string
	testPattern string
	testNum     int
}

type progressTracker struct {
	mu         sync.Mutex
	trackers   map[string]*versionProgress
	lastUpdate time.Time
}

type versionProgress struct {
	dbType        string
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

func main() {
	// Get test pattern from command line args, default to "WithTestcontainers"
	testPattern := "WithTestcontainers"
	if len(os.Args) > 1 {
		testPattern = os.Args[1]
	}

	// Get parallelism from environment variable
	parallel := getParallelism()

	fmt.Printf("Testcontainers Matrix Test Suite\n")
	fmt.Printf("Test pattern: %s | Parallelism: %d\n\n", testPattern, parallel)

	// Build all test jobs
	var jobs []testJob
	testNum := 0

	// MySQL tests
	for _, version := range mysqlVersions {
		testNum++
		jobs = append(jobs, testJob{
			image:       version,
			dbType:      "MySQL",
			testPattern: testPattern,
			testNum:     testNum,
		})
	}

	// Percona tests
	for _, version := range perconaVersions {
		testNum++
		jobs = append(jobs, testJob{
			image:       version,
			dbType:      "Percona",
			testPattern: testPattern,
			testNum:     testNum,
		})
	}

	// MariaDB tests
	for _, version := range mariadbVersions {
		testNum++
		jobs = append(jobs, testJob{
			image:       version,
			dbType:      "MariaDB",
			testPattern: testPattern,
			testNum:     testNum,
		})
	}

	// TiDB tests
	for _, version := range tidbVersions {
		testNum++
		jobs = append(jobs, testJob{
			image:       version,
			dbType:      "TiDB",
			testPattern: testPattern,
			testNum:     testNum,
		})
	}

	totalJobsCount = len(jobs)

	// Run tests (sequentially or in parallel)
	var results []testResult
	if parallel > 1 {
		isParallel = true
		results = runTestsParallel(jobs, parallel)
	} else {
		isParallel = false
		results = runTestsSequential(jobs)
	}

	// Print summary
	printSummary(results)

	// Exit with error code if any tests failed
	for _, result := range results {
		if !result.passed {
			os.Exit(1)
		}
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

func runTestsSequential(jobs []testJob) []testResult {
	var results []testResult

	for _, job := range jobs {
		result := runTest(job)
		results = append(results, result)
	}

	return results
}

func runTestsParallel(jobs []testJob, parallel int) []testResult {
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
	if !isParallel {
		// In sequential mode, show counter and start progress bar
		fmt.Printf("\n[%d/%d] ", job.testNum, getTotalJobs())
		progress.mu.Lock()
		tracker := progress.trackers[key]
		progress.mu.Unlock()
		renderProgress(tracker)
	}
	outputMutex.Unlock()

	// Sanitize image name for log file
	logFile := fmt.Sprintf("/tmp/testcontainers-%s-%s.log", job.dbType, sanitizeImageName(job.image))

	start := time.Now()

	// Build the go test command with JSON output
	cmd := exec.Command("go", "test",
		"-tags=testcontainers",
		"-json",
		"./mysql/...",
		"-run", job.testPattern,
		"-timeout", "15m",
	)

	// Set environment variables
	envVars := os.Environ()
	// All database types use DOCKER_IMAGE
	// For TiDB, format is tidb:VERSION (e.g., tidb:6.1.7)
	// For MySQL/Percona/MariaDB, format is already full image name (e.g., mysql:8.0)
	dockerImage := job.image
	if job.dbType == "TiDB" {
		// TiDB image is just version number, prepend "tidb:" prefix
		dockerImage = "tidb:" + job.image
	}
	envVars = append(envVars, "DOCKER_IMAGE="+dockerImage)
	envVars = append(envVars, "TF_ACC=1", "GOTOOLCHAIN=auto")
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
						parseTestOutput(lines[i], key)
					}
				}
			}
			if readErr != nil {
				// Process any remaining line in buffer
				if lineBuffer.Len() > 0 {
					parseTestOutput(lineBuffer.String(), key)
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

		// Render final progress state
		if tracker.totalTests > 0 {
			outputMutex.Lock()
			if !isParallel {
				// Clear the in-progress line
				fmt.Fprintf(os.Stderr, "\r\033[K")
			}
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

func parseTestOutput(line string, key string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	var event testEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Not valid JSON, skip
		return
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
	if tracker.totalTests > 0 && (tracker.passedTests > 0 || tracker.failedTests > 0) {
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

func printSummary(results []testResult) {
	fmt.Println()

	total := len(results)
	passed := 0
	failed := 0

	// Sort results by database type and version for better readability
	sortedResults := make([]testResult, len(results))
	copy(sortedResults, results)
	sort.Slice(sortedResults, func(i, j int) bool {
		if sortedResults[i].dbType != sortedResults[j].dbType {
			return sortedResults[i].dbType < sortedResults[j].dbType
		}
		return sortedResults[i].image < sortedResults[j].image
	})

	// Create table
	table := tablewriter.NewWriter(os.Stdout)
	table.Options(
		tablewriter.WithHeader([]string{"Database", "Version", "Status", "Duration"}),
	)

	// Add rows
	for _, result := range sortedResults {
		status := "PASS"
		if !result.passed {
			status = "FAIL"
			failed++
		} else {
			passed++
		}

		// Extract version from image (e.g., "mysql:8.0" -> "8.0")
		version := extractVersion(result.image)
		duration := formatDuration(result.duration)

		// Add test counts to status
		if result.totalTests > 0 {
			if result.failedTests > 0 {
				status = fmt.Sprintf("%s (%d/%d, %d failed)", status, result.passedTests, result.totalTests, result.failedTests)
			} else {
				status = fmt.Sprintf("%s (%d/%d)", status, result.passedTests, result.totalTests)
			}
		}

		row := []string{
			result.dbType,
			version,
			status,
			duration,
		}
		table.Append(row)
	}

	table.Render()

	fmt.Printf("\nSummary: %d/%d passed", passed, total)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
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
