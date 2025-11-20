//go:build testcontainers
// +build testcontainers

package mysql

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// To run these acceptance tests, you will need access to a MySQL server.
// Amazon RDS is one way to get a MySQL server. If you use RDS, you can
// use the root account credentials you specified when creating an RDS
// instance to get the access necessary to run these tests. (the tests
// assume full access to the server.)
//
// Set the MYSQL_ENDPOINT and MYSQL_USERNAME environment variables before
// running the tests. If the given user has a password then you will also need
// to set MYSQL_PASSWORD.
//
// The tests assume a reasonably-vanilla MySQL configuration. In particular,
// they assume that the "utf8" character set is available and that
// "utf8_bin" is a valid collation that isn't the default for that character
// set.
//
// You can run the tests like this:
//    make testacc TEST=./builtin/providers/mysql

var testAccProviderFactories map[string]func() (*schema.Provider, error)

// var testAccProviders map[string]*schema.Provider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviderFactories = map[string]func() (*schema.Provider, error){
		"mysql": func() (*schema.Provider, error) { return testAccProvider, nil },
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = Provider()
}

// TestMain sets up a shared MySQL/TiDB container for all testcontainers tests
// This is more efficient than starting a container for each test
func TestMain(m *testing.M) {
	// Force stderr to be unbuffered so debug output appears immediately
	os.Stderr.WriteString("TestMain: ENTRY POINT REACHED\n")
	os.Stderr.Sync()

	// Require DOCKER_IMAGE to be set - fail early if missing
	dockerImage := os.Getenv("DOCKER_IMAGE")
	os.Stderr.WriteString(fmt.Sprintf("TestMain: DOCKER_IMAGE='%s'\n", dockerImage))
	os.Stderr.Sync()

	if dockerImage == "" {
		os.Stderr.WriteString("ERROR: DOCKER_IMAGE environment variable is not set.\n")
		os.Stderr.WriteString("Please set DOCKER_IMAGE to the appropriate Docker image:\n")
		os.Stderr.WriteString("  - MySQL/Percona/MariaDB: mysql:5.6, percona:8.0, mariadb:10.10\n")
		os.Stderr.WriteString("  - TiDB: tidb:6.1.7, tidb:8.5.3\n")
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

func testAccPreCheck(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"MYSQL_ENDPOINT", "MYSQL_USERNAME"} {
		if v := os.Getenv(name); v == "" {
			t.Fatal("MYSQL_ENDPOINT, MYSQL_USERNAME and optionally MYSQL_PASSWORD must be set for acceptance tests")
		}
	}

	raw := map[string]interface{}{
		"conn_params": map[string]interface{}{},
	}
	err := testAccProvider.Configure(ctx, terraform.NewResourceConfigRaw(raw))
	if err != nil {
		t.Fatal(err)
	}
}

func testAccPreCheckSkipNotRds(t *testing.T) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		return
	}

	rdsEnabled, err := serverRds(db)
	if err != nil {
		return
	}

	if !rdsEnabled {
		t.Skip("Skip on non RDS instance")
	}
}

func testAccPreCheckSkipRds(t *testing.T) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		if strings.Contains(err.Error(), "SUPER privilege(s) for this operation") {
			t.Skip("Skip on RDS")
		}
		return
	}

	rdsEnabled, err := serverRds(db)
	if err != nil {
		return
	}

	if rdsEnabled {
		t.Skip("Skip on RDS")
	}
}

func testAccPreCheckSkipTiDB(t *testing.T) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipTiDB): %v", err)
		return
	}

	currentVersionString, err := serverVersionString(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipTiDB): %v", err)
		return
	}

	if strings.Contains(currentVersionString, "TiDB") {
		t.Skip("Skip on TiDB")
	}
}

func testAccPreCheckSkipMariaDB(t *testing.T) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipMariaDB): %v", err)
		return
	}

	currentVersionString, err := serverVersionString(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipMariaDB): %v", err)
		return
	}

	if strings.Contains(currentVersionString, "MariaDB") {
		t.Skip("Skip on MariaDB")
	}
}

func testAccPreCheckSkipNotMySQL8(t *testing.T) {
	testAccPreCheckSkipNotMySQLVersionMin(t, "8.0.0")
}

func testAccPreCheckSkipNotMySQLVersionMin(t *testing.T, minVersion string) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipNotMySQL8): %v", err)
		return
	}

	currentVersion, err := serverVersion(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipNotMySQL8): %v", err)
		return
	}

	versionMin, _ := version.NewVersion(minVersion)
	if currentVersion.LessThan(versionMin) {
		// TiDB 7.x series advertises as 8.0 mysql so we batch its testing strategy with Mysql8
		isTiDB, tidbVersion, mysqlCompatibilityVersion, err := serverTiDB(db)
		if err != nil {
			t.Fatalf("Cannot get DB version string (SkipNotMySQL8): %v", err)
			return
		}
		if isTiDB {
			mysqlVersion, err := version.NewVersion(mysqlCompatibilityVersion)
			if err != nil {
				t.Fatalf("Cannot get DB version string for TiDB (SkipNotMySQL8): %s %s %v", tidbVersion, mysqlCompatibilityVersion, err)
				return
			}
			if mysqlVersion.LessThan(versionMin) {
				t.Skip("Skip on MySQL8")
			}
		}

		t.Skip("Skip on MySQL8")
	}
}

func testAccPreCheckSkipNotTiDB(t *testing.T) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipNotTiDB): %v", err)
		return
	}

	currentVersionString, err := serverVersionString(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipNotTiDB): %v", err)
		return
	}

	if !strings.Contains(currentVersionString, "TiDB") {
		msg := fmt.Sprintf("Skip on MySQL %s", currentVersionString)
		t.Skip(msg)
	}
}

func testAccPreCheckSkipNotTiDBVersionMin(t *testing.T, minVersion string) {
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipNotTiDBVersionMin): %v", err)
		return
	}

	currentVersion, err := serverVersion(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipNotTiDBVersionMin): %v", err)
		return
	}

	versionMin, _ := version.NewVersion(minVersion)
	if currentVersion.LessThan(versionMin) {
		isTiDB, tidbVersion, _, err := serverTiDB(db)
		if err != nil {
			t.Fatalf("Cannot get DB version string (SkipNotTiDBVersionMin): %v", err)
			return
		}
		if isTiDB {
			tidbSemVar, err := version.NewVersion(tidbVersion)
			if err != nil {
				t.Fatalf("Cannot get DB version string for TiDB (SkipNotTiDBVersionMin): %s %v", tidbSemVar, err)
				return
			}
			if tidbSemVar.LessThan(versionMin) {
				t.Skip("Skip on TiDB (SkipNotTiDBVersionMin)")
			}
			return
		}

		t.Skip("Skip on MySQL")
	}
}
