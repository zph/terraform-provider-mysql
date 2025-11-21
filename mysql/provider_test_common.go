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

func testAccPreCheck(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"MYSQL_ENDPOINT", "MYSQL_USERNAME"} {
		if v := os.Getenv(name); v == "" {
			// If container failed to start, allow tests to skip gracefully
			// Check DOCKER_IMAGE to provide helpful error message
			dockerImage := os.Getenv("DOCKER_IMAGE")
			if dockerImage != "" {
				t.Fatalf("MYSQL_ENDPOINT not set - container may have failed to start for %s. Check TestMain logs.", dockerImage)
			}
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
	// Check if container startup failed - if so, skip (can't determine if RDS)
	containerStartupFailed := os.Getenv("CONTAINER_STARTUP_FAILED")
	if containerStartupFailed == "1" {
		t.Skip("Skip on RDS (container startup failed, cannot determine RDS status)")
	}

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
	// Early skip check based on DOCKER_IMAGE before connecting
	dockerImage := os.Getenv("DOCKER_IMAGE")
	if dockerImage != "" && strings.HasPrefix(dockerImage, "tidb:") {
		t.Skip("Skip on TiDB")
	}

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
	// Early skip check based on DOCKER_IMAGE before connecting
	dockerImage := os.Getenv("DOCKER_IMAGE")
	if dockerImage != "" && strings.HasPrefix(dockerImage, "mariadb:") {
		t.Skip("Skip on MariaDB")
	}

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
	// Early skip check based on DOCKER_IMAGE before connecting
	// This allows tests to skip even if TestMain failed to start the container
	dockerImage := os.Getenv("DOCKER_IMAGE")
	if dockerImage != "" {
		// Parse version from DOCKER_IMAGE (e.g., "mysql:5.6", "percona:5.7", "mariadb:10.3")
		parts := strings.Split(dockerImage, ":")
		if len(parts) == 2 {
			imageName := parts[0]
			imageVersion := parts[1]

			// Check if this is a MySQL/Percona version that's definitely too old
			if imageName == "mysql" || imageName == "percona" || strings.HasPrefix(imageName, "percona/") {
				versionMin, err := version.NewVersion(minVersion)
				if err == nil {
					// Try to parse the image version
					imgVer, err := version.NewVersion(imageVersion)
					if err == nil && imgVer.LessThan(versionMin) {
						t.Skipf("Skip on %s (requires MySQL %s+)", dockerImage, minVersion)
					}
				}
			}
			// MariaDB versions are typically 10.x, which are < 8.0, so skip if minVersion is 8.0+
			if imageName == "mariadb" {
				versionMin, err := version.NewVersion(minVersion)
				if err == nil {
					// MariaDB 10.x is roughly equivalent to MySQL 5.x in terms of features
					// So if we need MySQL 8.0+, skip MariaDB
					mysql80, _ := version.NewVersion("8.0.0")
					if versionMin.GreaterThanOrEqual(mysql80) {
						t.Skipf("Skip on %s (requires MySQL %s+)", dockerImage, minVersion)
					}
				}
			}
		}
	}

	// Check if container startup failed - if so, skip if we can determine incompatibility from DOCKER_IMAGE
	// Otherwise, try to connect (which will fail gracefully)
	containerStartupFailed := os.Getenv("CONTAINER_STARTUP_FAILED")
	if containerStartupFailed == "1" {
		// Container failed to start - if we already determined this version is incompatible, skip
		// Otherwise, we'll fail in testAccPreCheck which is fine
	}

	// Only call testAccPreCheck if we didn't skip above
	// This allows tests to skip even if container startup failed
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

func testAccPreCheckSkipNotMySQLVersionMax(t *testing.T, maxVersion string) {
	// Early skip check based on DOCKER_IMAGE before connecting
	// This allows tests to skip even if TestMain failed to start the container
	dockerImage := os.Getenv("DOCKER_IMAGE")
	if dockerImage != "" {
		// Parse version from DOCKER_IMAGE (e.g., "mysql:5.6", "percona:5.7", "mariadb:10.3")
		parts := strings.Split(dockerImage, ":")
		if len(parts) == 2 {
			imageName := parts[0]
			imageVersion := parts[1]

			// Check if this is a MySQL/Percona version that's too new
			if imageName == "mysql" || imageName == "percona" || strings.HasPrefix(imageName, "percona/") {
				versionMax, err := version.NewVersion(maxVersion)
				if err == nil {
					// Try to parse the image version
					imgVer, err := version.NewVersion(imageVersion)
					if err == nil && imgVer.GreaterThan(versionMax) {
						t.Skipf("Skip on %s (requires MySQL %s or older)", dockerImage, maxVersion)
					}
				}
			}
		}
	}

	// Only call testAccPreCheck if we didn't skip above
	testAccPreCheck(t)

	ctx := context.Background()
	db, err := connectToMySQL(ctx, testAccProvider.Meta().(*MySQLConfiguration))
	if err != nil {
		t.Fatalf("Cannot connect to DB (SkipNotMySQLVersionMax): %v", err)
		return
	}

	currentVersion, err := serverVersion(db)
	if err != nil {
		t.Fatalf("Cannot get DB version string (SkipNotMySQLVersionMax): %v", err)
		return
	}

	versionMax, _ := version.NewVersion(maxVersion)
	if currentVersion.GreaterThan(versionMax) {
		t.Skipf("Skip on MySQL %s (requires %s or older)", currentVersion.String(), maxVersion)
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
