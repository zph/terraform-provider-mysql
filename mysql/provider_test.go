//go:build testcontainers
// +build testcontainers

package mysql

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain sets up a shared MySQL/TiDB container for all testcontainers tests
// This is more efficient than starting a container for each test
func TestMain(m *testing.M) {
	// Force stderr to be unbuffered so debug output appears immediately
	os.Stderr.WriteString("TestMain: ENTRY POINT REACHED\n")
	os.Stderr.Sync()

	configureTestcontainersRuntime()

	// Require DOCKER_IMAGE to be set - fail early if missing
	dockerImage := os.Getenv("DOCKER_IMAGE")
	os.Stderr.WriteString(fmt.Sprintf("TestMain: DOCKER_IMAGE='%s'\n", dockerImage))
	os.Stderr.Sync()

	if dockerImage == "" {
		os.Stderr.WriteString("ERROR: DOCKER_IMAGE environment variable is not set.\n")
		os.Stderr.WriteString("Please set DOCKER_IMAGE to the appropriate Docker image:\n")
		os.Stderr.WriteString("  - MySQL/Percona/MariaDB: mysql:5.6, percona:8.0, mariadb:10.10\n")
		os.Stderr.WriteString("  - TiDB: tidb:6.1.7, tidb:8.5.5\n")
		os.Exit(1)
	}

	// Debug: Log that TestMain is running
	os.Stderr.WriteString(fmt.Sprintf("TestMain: Starting with DOCKER_IMAGE='%s'\n", dockerImage))
	os.Stderr.Sync()

	// Check if we're testing TiDB (format: tidb:VERSION)
	// TiDB requires multi-container setup
	if strings.HasPrefix(dockerImage, "tidb:") {
		tidbVersion := strings.TrimPrefix(dockerImage, "tidb:")
		if tidbVersion == "" {
			os.Stderr.WriteString("ERROR: DOCKER_IMAGE format for TiDB must be 'tidb:VERSION' (e.g., tidb:6.1.7)\n")
			os.Exit(1)
		}

		// Start shared TiDB cluster before running tests
		var err error
		sharedTiDBClusterMtx.Lock()
		sharedTiDBCluster, err = startSharedTiDBCluster(tidbVersion)
		sharedTiDBClusterMtx.Unlock()

		if err != nil {
			// If cluster startup fails, exit with error
			os.Stderr.WriteString(fmt.Sprintf("Failed to start shared TiDB cluster: %v\n", err))
			os.Exit(1)
		}

		// Set up environment variables for the shared TiDB cluster
		os.Setenv("MYSQL_ENDPOINT", sharedTiDBCluster.Endpoint)
		os.Setenv("MYSQL_USERNAME", sharedTiDBCluster.Username)
		os.Setenv("MYSQL_PASSWORD", sharedTiDBCluster.Password)

		// Run all tests
		code := m.Run()

		// Cleanup shared TiDB cluster after all tests complete
		cleanupSharedTiDBCluster()

		// Exit with test result code
		os.Exit(code)
	}

	// MySQL/Percona/MariaDB mode - use single container
	// Start shared container before running tests
	var err error
	sharedContainerMtx.Lock()
	sharedContainer, err = startSharedMySQLContainer(dockerImage)
	sharedContainerMtx.Unlock()

	if err != nil {
		// If container startup fails, exit with error
		os.Stderr.WriteString(fmt.Sprintf("Failed to start shared MySQL container: %v\n", err))
		os.Exit(1)
	}

	// Set up environment variables for the shared container
	// These MUST be set even if sharedContainer is nil (shouldn't happen, but be defensive)
	if sharedContainer != nil {
		os.Setenv("MYSQL_ENDPOINT", sharedContainer.Endpoint)
		os.Setenv("MYSQL_USERNAME", sharedContainer.Username)
		os.Setenv("MYSQL_PASSWORD", sharedContainer.Password)
		// Debug: Log that environment variables are set
		os.Stderr.WriteString(fmt.Sprintf("TestMain: Set MYSQL_ENDPOINT='%s'\n", sharedContainer.Endpoint))
		os.Stderr.Sync()
	} else {
		// This should never happen, but if it does, fail loudly
		os.Stderr.WriteString(fmt.Sprintf("ERROR: startSharedMySQLContainer returned nil container without error for image '%s'\n", dockerImage))
		os.Exit(1)
	}

	// Run all tests
	code := m.Run()

	// Cleanup shared container after all tests complete
	cleanupSharedContainer()

	// Exit with test result code
	os.Exit(code)
}

func configureTestcontainersRuntime() {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") != "" {
		return
	}

	if strings.Contains(os.Getenv("DOCKER_HOST"), "podman.sock") {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		os.Stderr.WriteString("TestMain: detected Podman socket; setting TESTCONTAINERS_RYUK_DISABLED=true\n")
		os.Stderr.Sync()
	}
}
