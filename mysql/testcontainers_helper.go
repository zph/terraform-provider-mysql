//go:build testcontainers
// +build testcontainers

// Suppress warnings from go-m1cpu dependency
// These are harmless compiler warnings from CGO code in a third-party package

package mysql

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedContainer     *MySQLTestContainer
	sharedContainerOnce sync.Once
	sharedContainerMtx  sync.Mutex

	sharedTiDBCluster     *TiDBTestCluster
	sharedTiDBClusterOnce sync.Once
	sharedTiDBClusterMtx  sync.Mutex
)

func init() {
	// Suppress MySQL driver "unexpected EOF" log messages during tests
	// These are harmless connection cleanup messages that occur when connections
	// are closed during test cleanup
	mysql.SetLogger(log.New(&mysqlLogFilter{Writer: io.Discard}, "", log.LstdFlags))
}

// mysqlLogFilter filters out "unexpected EOF" messages from MySQL driver logs
type mysqlLogFilter struct {
	io.Writer
}

func (f *mysqlLogFilter) Write(p []byte) (n int, err error) {
	// Filter out "unexpected EOF" messages
	if strings.Contains(string(p), "unexpected EOF") {
		return len(p), nil // Discard the message
	}
	return len(p), nil // Also discard other messages to suppress all MySQL driver logging
}

// MySQLTestContainer wraps a testcontainers MySQL container with connection details
type MySQLTestContainer struct {
	Container testcontainers.Container
	Endpoint  string
	Username  string
	Password  string
}

// startMySQLContainer starts a MySQL/Percona/MariaDB container for testing
// Supports MySQL, Percona, and MariaDB images
// image must not be empty - function will panic if empty
func startMySQLContainer(ctx context.Context, t *testing.T, image string) *MySQLTestContainer {
	if image == "" {
		t.Fatalf("ERROR: startMySQLContainer called with empty image. DOCKER_IMAGE must be set.")
	}
	// Determine timeout based on image/version
	timeout := 120 * time.Second
	if contains(image, "5.6") || contains(image, "5.7") || contains(image, "6.1") || contains(image, "6.5") {
		// Older versions may need more time
		timeout = 180 * time.Second
	}

	// Use GenericContainer for compatibility with Go 1.21
	// Configure MySQL with environment variables
	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD":        "",
			"MYSQL_ALLOW_EMPTY_PASSWORD": "1",
			"MYSQL_DATABASE":             "testdb",
		},
		WaitingFor: wait.ForLog("ready for connections").
			WithOccurrence(2).
			WithStartupTimeout(timeout),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start MySQL container (%s): %v", image, err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	return &MySQLTestContainer{
		Container: container,
		Endpoint:  endpoint,
		Username:  "root",
		Password:  "",
	}
}

// SetupTestEnv sets environment variables for the test and returns a cleanup function
func (m *MySQLTestContainer) SetupTestEnv(t *testing.T) func() {
	originalEndpoint := os.Getenv("MYSQL_ENDPOINT")
	originalUsername := os.Getenv("MYSQL_USERNAME")
	originalPassword := os.Getenv("MYSQL_PASSWORD")

	os.Setenv("MYSQL_ENDPOINT", m.Endpoint)
	os.Setenv("MYSQL_USERNAME", m.Username)
	os.Setenv("MYSQL_PASSWORD", m.Password)

	return func() {
		// Restore original values or unset
		if originalEndpoint != "" {
			os.Setenv("MYSQL_ENDPOINT", originalEndpoint)
		} else {
			os.Unsetenv("MYSQL_ENDPOINT")
		}
		if originalUsername != "" {
			os.Setenv("MYSQL_USERNAME", originalUsername)
		} else {
			os.Unsetenv("MYSQL_USERNAME")
		}
		if originalPassword != "" {
			os.Setenv("MYSQL_PASSWORD", originalPassword)
		} else {
			os.Unsetenv("MYSQL_PASSWORD")
		}

		// Terminate container
		ctx := context.Background()
		if err := m.Container.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate container: %v", err)
		}
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// getSharedMySQLContainer returns the shared MySQL container set up by TestMain
// The image parameter is ignored - TestMain uses DOCKER_IMAGE env var
// This function validates that DOCKER_IMAGE is set and fails early if not
// For TiDB tests, TestMain already sets up the cluster and environment variables,
// so this function just validates the environment is ready
func getSharedMySQLContainer(t *testing.T, image string) *MySQLTestContainer {
	// Validate that DOCKER_IMAGE is set (required by TestMain)
	// This validation must always be present - fail early if DOCKER_IMAGE is empty
	dockerImage := os.Getenv("DOCKER_IMAGE")
	if dockerImage == "" {
		t.Fatalf("ERROR: DOCKER_IMAGE environment variable is not set.\n" +
			"Please set DOCKER_IMAGE to the appropriate Docker image:\n" +
			"  - MySQL/Percona/MariaDB: mysql:5.6, percona:8.0, mariadb:10.10\n" +
			"  - TiDB: tidb:6.1.7, tidb:8.5.3\n" +
			"The 'image' parameter to getSharedMySQLContainer is ignored - use DOCKER_IMAGE env var instead.")
	}

	// Validate that the provided image matches DOCKER_IMAGE (if provided)
	if image != "" && image != dockerImage {
		t.Fatalf("ERROR: getSharedMySQLContainer called with image '%s' but DOCKER_IMAGE is set to '%s'.\n"+
			"Remove the hardcoded image parameter - TestMain uses DOCKER_IMAGE env var to create the shared container.",
			image, dockerImage)
	}

	// Check if we're in TiDB mode
	// For TiDB, TestMain already set up the cluster and environment variables
	// Just validate that the environment variables are set and return a dummy container
	if strings.HasPrefix(dockerImage, "tidb:") {
		// Validate that TestMain set up the environment variables
		endpoint := os.Getenv("MYSQL_ENDPOINT")
		if endpoint == "" {
			t.Fatalf("ERROR: MYSQL_ENDPOINT not set. TestMain should have set this for TiDB cluster.")
		}
		// Return a dummy container - tests will use environment variables set by TestMain
		return &MySQLTestContainer{
			Container: nil, // Not used for TiDB
			Endpoint:  endpoint,
			Username:  os.Getenv("MYSQL_USERNAME"),
			Password:  os.Getenv("MYSQL_PASSWORD"),
		}
	}

	// For MySQL/Percona/MariaDB, TestMain should have set MYSQL_ENDPOINT
	// Use environment variables as the source of truth (TestMain always sets these)
	endpoint := os.Getenv("MYSQL_ENDPOINT")
	if endpoint == "" {
		t.Fatalf("ERROR: MYSQL_ENDPOINT not set. TestMain should have set this using DOCKER_IMAGE='%s'.\n"+
			"This indicates TestMain did not run or failed to initialize.", dockerImage)
	}

	// If sharedContainer is available, use it; otherwise use environment variables
	if sharedContainer != nil {
		return sharedContainer
	}

	// Fallback: use environment variables set by TestMain
	// This handles cases where sharedContainer might be nil but environment variables are set
	return &MySQLTestContainer{
		Container: nil, // Not available, but tests use environment variables
		Endpoint:  endpoint,
		Username:  os.Getenv("MYSQL_USERNAME"),
		Password:  os.Getenv("MYSQL_PASSWORD"),
	}
}

// startSharedMySQLContainer starts a shared MySQL container without requiring a testing.T
// Used by TestMain for initial setup
// image must not be empty - function will return error if empty
func startSharedMySQLContainer(image string) (*MySQLTestContainer, error) {
	if image == "" {
		return nil, fmt.Errorf("ERROR: startSharedMySQLContainer called with empty image. DOCKER_IMAGE must be set")
	}
	ctx := context.Background()

	// Determine timeout based on image/version
	timeout := 120 * time.Second
	if contains(image, "5.6") || contains(image, "5.7") || contains(image, "6.1") || contains(image, "6.5") {
		timeout = 180 * time.Second
	}

	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD":        "",
			"MYSQL_ALLOW_EMPTY_PASSWORD": "1",
			"MYSQL_DATABASE":             "testdb",
		},
		WaitingFor: wait.ForLog("ready for connections").
			WithOccurrence(2).
			WithStartupTimeout(timeout),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start MySQL container (%s): %v", image, err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3306")
	if err != nil {
		return nil, fmt.Errorf("failed to get container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	mysqlContainer := &MySQLTestContainer{
		Container: container,
		Endpoint:  endpoint,
		Username:  "root",
		Password:  "",
	}

	// Install mysql_no_login plugin (required for some tests)
	// This matches what the Makefile does
	// Wait a bit for MySQL to be fully ready before installing plugin
	time.Sleep(2 * time.Second)
	if err := installMySQLNoLoginPlugin(ctx, mysqlContainer); err != nil {
		// Log warning but don't fail - plugin may already be installed or not available
		// Some MySQL versions/distributions may not have this plugin
		fmt.Printf("Warning: Could not install mysql_no_login plugin: %v (some tests may skip)\n", err)
	}

	return mysqlContainer, nil
}

// installMySQLNoLoginPlugin installs the mysql_no_login plugin in the container
func installMySQLNoLoginPlugin(ctx context.Context, container *MySQLTestContainer) error {
	// Connect to MySQL and install the plugin
	db, err := connectToMySQL(ctx, &MySQLConfiguration{
		Config: &mysql.Config{
			User:   container.Username,
			Passwd: container.Password,
			Net:    "tcp",
			Addr:   container.Endpoint,
		},
		MaxConnLifetime:        0,
		MaxOpenConns:           1,
		ConnectRetryTimeoutSec: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to install plugin: %v", err)
	}
	defer db.Close()

	// Try to install the plugin (ignore error if already installed or not available)
	_, err = db.ExecContext(ctx, "INSTALL PLUGIN mysql_no_login SONAME 'mysql_no_login.so'")
	if err != nil {
		errStr := err.Error()
		// Ignore if plugin already exists or if plugin file doesn't exist (MySQL 8.0 may not have it)
		if strings.Contains(errStr, "already exists") ||
			strings.Contains(errStr, "file not found") ||
			strings.Contains(errStr, "does not exist") {
			return nil // Not an error - plugin already installed or not available
		}
		return fmt.Errorf("failed to install mysql_no_login plugin: %v", err)
	}

	return nil
}

// cleanupSharedContainer terminates the shared container
func cleanupSharedContainer() {
	sharedContainerMtx.Lock()
	defer sharedContainerMtx.Unlock()

	if sharedContainer != nil {
		ctx := context.Background()
		if err := sharedContainer.Container.Terminate(ctx); err != nil {
			// Use fmt.Printf since we're in cleanup and testing.T may not be available
			fmt.Printf("Warning: Failed to terminate shared container: %v\n", err)
		}
		sharedContainer = nil
	}

	// Clean up environment variables
	os.Unsetenv("MYSQL_ENDPOINT")
	os.Unsetenv("MYSQL_USERNAME")
	os.Unsetenv("MYSQL_PASSWORD")
}

// TiDBTestCluster wraps TiDB cluster containers with connection details
type TiDBTestCluster struct {
	PDContainer   testcontainers.Container
	TiKVContainer testcontainers.Container
	TiDBContainer testcontainers.Container
	Network       testcontainers.Network
	Endpoint      string
	Username      string
	Password      string
	// PlaygroundContainer is used when TiUP Playground is used instead of separate containers
	PlaygroundContainer testcontainers.Container
}

// startTiDBCluster starts a TiDB cluster (PD, TiKV, TiDB) for testing
// TiDB requires a multi-container setup with coordination between components
func startTiDBCluster(ctx context.Context, t *testing.T, version string) *TiDBTestCluster {
	// Create a Docker network for TiDB cluster components
	testNetwork, err := network.New(ctx,
		network.WithCheckDuplicate(),
		network.WithDriver("bridge"),
	)
	if err != nil {
		t.Fatalf("Failed to create Docker network: %v", err)
	}

	networkName := testNetwork.Name

	// Start PD (Placement Driver) - must start first
	pdContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/pd:v%s", version),
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"pd"}},
			Cmd: []string{
				"--name=pd",
				"--data-dir=/data",
				"--client-urls=http://0.0.0.0:2379",
				"--advertise-client-urls=http://pd:2379",
				"--peer-urls=http://0.0.0.0:2380",
				"--advertise-peer-urls=http://pd:2380",
				"--initial-cluster=pd=http://pd:2380",
			},
			WaitingFor: wait.ForLog("ready to serve").
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start PD container: %v", err)
	}

	// Start TiKV (storage layer) - connects to PD
	// TiKV requires increased file descriptor limit
	// v8.x versions require at least 123880, older versions require at least 82920
	tikvFdLimit := 200000 // Default for older versions
	if strings.HasPrefix(version, "8.") {
		// TiDB v8.x requires higher file descriptor limit
		tikvFdLimit = 250000
	}

	tikvContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/tikv:v%s", version),
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"tikv"}},
			Cmd: []string{
				"--addr=0.0.0.0:20160",
				"--advertise-addr=tikv:20160",
				"--status-addr=0.0.0.0:20180",
				"--data-dir=/data",
				"--pd=pd:2379",
			},
			HostConfigModifier: func(hostConfig *container.HostConfig) {
				// Set ulimit for file descriptors (v8.x requires at least 123880)
				hostConfig.Ulimits = []*container.Ulimit{
					{
						Name: "nofile",
						Soft: int64(tikvFdLimit),
						Hard: int64(tikvFdLimit),
					},
				}
			},
			WaitingFor: wait.ForAll(
				// Wait for TiKV to connect to PD and start serving
				wait.ForLog("succeed to update max timestamp").
					WithOccurrence(3), // Wait for at least 3 region updates
				wait.ForListeningPort("20180/tcp"), // Status port
			).WithStartupTimeout(180 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start TiKV container: %v", err)
	}

	// Start TiDB (SQL layer) - connects to PD, uses TiKV for storage
	tidbContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/tidb:v%s", version),
			ExposedPorts:   []string{"4000/tcp"},
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"tidb"}},
			Cmd: []string{
				"--store=tikv",
				"-P", "4000",
				"--path=pd:2379",
			},
			WaitingFor: wait.ForLog("server is running MySQL protocol").
				WithOccurrence(1).
				WithStartupTimeout(180 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start TiDB container: %v", err)
	}

	// Get TiDB endpoint
	host, err := tidbContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get TiDB container host: %v", err)
	}

	port, err := tidbContainer.MappedPort(ctx, "4000")
	if err != nil {
		t.Fatalf("Failed to get TiDB container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	return &TiDBTestCluster{
		PDContainer:   pdContainer,
		TiKVContainer: tikvContainer,
		TiDBContainer: tidbContainer,
		Network:       testNetwork,
		Endpoint:      endpoint,
		Username:      "root",
		Password:      "",
	}
}

// startSharedTiDBClusterWithTiUP starts a TiDB cluster using TiUP Playground inside a single container
// This is faster and simpler than managing separate PD, TiKV, and TiDB containers
func startSharedTiDBClusterWithTiUP(version string) (*TiDBTestCluster, error) {
	ctx := context.Background()

	// Build TiUP Playground image from Dockerfile
	// This builds a container with TiUP installed that can run playground
	// Get the git root directory (where Dockerfile.tiup-playground is located)
	moduleRoot := os.Getenv("GITHUB_WORKSPACE")
	if moduleRoot == "" {
		// For local development, find git root using git rev-parse
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to find git root: %v", err)
		}
		moduleRoot = strings.TrimSpace(string(output))
	}

	// Verify Dockerfile exists
	dockerfilePath := filepath.Join(moduleRoot, "Dockerfile.tiup-playground")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, fmt.Errorf("Dockerfile.tiup-playground not found at %s: %v", dockerfilePath, err)
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:       moduleRoot,
			Dockerfile:    "Dockerfile.tiup-playground",
			PrintBuildLog: true, // Helpful for debugging
		},
		ExposedPorts: []string{"4000/tcp"},
		// TiUP Playground needs to run processes, so we need privileged mode
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Privileged = true
			// Set ulimit for file descriptors (TiKV inside playground needs this)
			hostConfig.Ulimits = []*container.Ulimit{
				{
					Name: "nofile",
					Soft: 250000,
					Hard: 250000,
				},
			}
		},
		Cmd: []string{
			"/root/.tiup/bin/tiup", "playground", version,
			"--db", "1",
			"--kv", "1",
			"--pd", "1",
			"--tiflash", "0",
			"--without-monitor",
			"--host", "0.0.0.0",
			"--db.port", "4000",
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("4000/tcp"),
			wait.ForSQL(nat.Port("4000/tcp"), "mysql", func(host string, port nat.Port) string {
				return fmt.Sprintf("root@tcp(%s:%s)/", host, port.Port())
			}),
		).WithStartupTimeout(240 * time.Second), // Longer timeout for first-time TiUP component downloads
	}

	playgroundContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start TiUP Playground container: %v", err)
	}

	// Get endpoint
	host, err := playgroundContainer.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get playground container host: %v", err)
	}

	port, err := playgroundContainer.MappedPort(ctx, "4000")
	if err != nil {
		return nil, fmt.Errorf("failed to get playground container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	return &TiDBTestCluster{
		PlaygroundContainer: playgroundContainer,
		Endpoint:            endpoint,
		Username:            "root",
		Password:            "",
	}, nil
}

// startSharedTiDBCluster starts a shared TiDB cluster without requiring a testing.T
// Used by TestMain for initial setup
// Now uses TiUP Playground for better performance and reliability
func startSharedTiDBCluster(version string) (*TiDBTestCluster, error) {
	// Use TiUP Playground approach - much faster and simpler
	return startSharedTiDBClusterWithTiUP(version)
}

// Legacy multi-container approach (kept for reference, but not used)
func startSharedTiDBClusterLegacy(version string) (*TiDBTestCluster, error) {
	ctx := context.Background()

	// Create a Docker network for TiDB cluster components
	testNetwork, err := network.New(ctx,
		network.WithCheckDuplicate(),
		network.WithDriver("bridge"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker network: %v", err)
	}

	networkName := testNetwork.Name

	// Start PD (Placement Driver) - must start first
	pdContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/pd:v%s", version),
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"pd"}},
			Cmd: []string{
				"--name=pd",
				"--data-dir=/data",
				"--client-urls=http://0.0.0.0:2379",
				"--advertise-client-urls=http://pd:2379",
				"--peer-urls=http://0.0.0.0:2380",
				"--advertise-peer-urls=http://pd:2380",
				"--initial-cluster=pd=http://pd:2380",
			},
			WaitingFor: wait.ForLog("ready to serve").
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start PD container: %v", err)
	}

	// Start TiKV (storage layer) - connects to PD
	// TiKV requires increased file descriptor limit
	// v8.x versions require at least 123880, older versions require at least 82920
	tikvFdLimit := 200000 // Default for older versions
	if strings.HasPrefix(version, "8.") {
		// TiDB v8.x requires higher file descriptor limit
		tikvFdLimit = 250000
	}

	tikvContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/tikv:v%s", version),
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"tikv"}},
			Cmd: []string{
				"--addr=0.0.0.0:20160",
				"--advertise-addr=tikv:20160",
				"--status-addr=0.0.0.0:20180",
				"--data-dir=/data",
				"--pd=pd:2379",
			},
			HostConfigModifier: func(hostConfig *container.HostConfig) {
				// Set ulimit for file descriptors (v8.x requires at least 123880)
				hostConfig.Ulimits = []*container.Ulimit{
					{
						Name: "nofile",
						Soft: int64(tikvFdLimit),
						Hard: int64(tikvFdLimit),
					},
				}
			},
			WaitingFor: wait.ForAll(
				// Wait for TiKV to connect to PD and start serving
				wait.ForLog("succeed to update max timestamp").
					WithOccurrence(3), // Wait for at least 3 region updates
				wait.ForListeningPort("20180/tcp"), // Status port
			).WithStartupTimeout(180 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start TiKV container: %v", err)
	}

	// Start TiDB (SQL layer) - connects to PD, uses TiKV for storage
	tidbContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          fmt.Sprintf("pingcap/tidb:v%s", version),
			ExposedPorts:   []string{"4000/tcp"},
			Networks:       []string{networkName},
			NetworkAliases: map[string][]string{networkName: {"tidb"}},
			Cmd: []string{
				"--store=tikv",
				"-P", "4000",
				"--path=pd:2379",
			},
			WaitingFor: wait.ForLog("server is running MySQL protocol").
				WithOccurrence(1).
				WithStartupTimeout(180 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start TiDB container: %v", err)
	}

	// Get TiDB endpoint
	host, err := tidbContainer.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TiDB container host: %v", err)
	}

	port, err := tidbContainer.MappedPort(ctx, "4000")
	if err != nil {
		return nil, fmt.Errorf("failed to get TiDB container port: %v", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	return &TiDBTestCluster{
		PDContainer:   pdContainer,
		TiKVContainer: tikvContainer,
		TiDBContainer: tidbContainer,
		Network:       testNetwork,
		Endpoint:      endpoint,
		Username:      "root",
		Password:      "",
	}, nil
}

// cleanupSharedTiDBCluster terminates the shared TiDB cluster
func cleanupSharedTiDBCluster() {
	sharedTiDBClusterMtx.Lock()
	defer sharedTiDBClusterMtx.Unlock()

	if sharedTiDBCluster != nil {
		ctx := context.Background()

		// If using TiUP Playground (single container)
		if sharedTiDBCluster.PlaygroundContainer != nil {
			if err := sharedTiDBCluster.PlaygroundContainer.Terminate(ctx); err != nil {
				fmt.Printf("Warning: Failed to terminate TiUP Playground container: %v\n", err)
			}
		} else {
			// Legacy multi-container approach
			if sharedTiDBCluster.TiDBContainer != nil {
				if err := sharedTiDBCluster.TiDBContainer.Terminate(ctx); err != nil {
					fmt.Printf("Warning: Failed to terminate TiDB container: %v\n", err)
				}
			}
			if sharedTiDBCluster.TiKVContainer != nil {
				if err := sharedTiDBCluster.TiKVContainer.Terminate(ctx); err != nil {
					fmt.Printf("Warning: Failed to terminate TiKV container: %v\n", err)
				}
			}
			if sharedTiDBCluster.PDContainer != nil {
				if err := sharedTiDBCluster.PDContainer.Terminate(ctx); err != nil {
					fmt.Printf("Warning: Failed to terminate PD container: %v\n", err)
				}
			}
			if sharedTiDBCluster.Network != nil {
				if err := sharedTiDBCluster.Network.Remove(ctx); err != nil {
					fmt.Printf("Warning: Failed to remove TiDB network: %v\n", err)
				}
			}
		}
		sharedTiDBCluster = nil
	}

	// Clean up environment variables
	os.Unsetenv("MYSQL_ENDPOINT")
	os.Unsetenv("MYSQL_USERNAME")
	os.Unsetenv("MYSQL_PASSWORD")
}
