package testmatrix

import (
	"fmt"
	"strings"
)

type Database string

const (
	MySQL   Database = "mysql"
	Percona Database = "percona"
	MariaDB Database = "mariadb"
	TiDB    Database = "tidb"
)

const (
	ImageMySQLPrefix             = "mysql:"
	ImagePerconaPrefix           = "percona:"
	ImagePerconaRepoPrefix       = "percona/"
	ImagePerconaServerPrefix     = "percona/percona-server:"
	ImageDockerPerconaRepoPrefix = "docker.io/percona/"
	ImageMariaDBPrefix           = "mariadb:"
	ImageTiDBPrefix              = "tidb:"

	VersionMySQL57   = "5.7"
	VersionPercona57 = "5.7"
	VersionPercona80 = "8.0"
	VersionPercona84 = "8.4"
)

type Entry struct {
	Database    Database
	Cycle       string
	Version     string
	DockerRepo  string
	TagPrefix   string
	BuildSuffix bool
	EOLProduct  string
	EOLCycle    string
	EOLProxyFor string
}

var Entries = []Entry{
	{Database: MySQL, Cycle: "8.0", Version: "8.0.46", DockerRepo: "library/mysql", EOLProduct: "mysql", EOLCycle: "8.0"},
	{Database: MySQL, Cycle: "8.4", Version: "8.4.9", DockerRepo: "library/mysql", EOLProduct: "mysql", EOLCycle: "8.4"},
	{Database: MySQL, Cycle: "9.7", Version: "9.7.0", DockerRepo: "library/mysql", EOLProduct: "mysql", EOLCycle: "9.7"},
	{Database: Percona, Cycle: "8.0", Version: "8.0.46-37", DockerRepo: "percona/percona-server", BuildSuffix: true, EOLProduct: "mysql", EOLCycle: "8.0", EOLProxyFor: "Percona Server"},
	{Database: Percona, Cycle: "8.4", Version: "8.4.8-8", DockerRepo: "percona/percona-server", BuildSuffix: true, EOLProduct: "mysql", EOLCycle: "8.4", EOLProxyFor: "Percona Server"},
	{Database: MariaDB, Cycle: "10.11", Version: "10.11.18", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "10.11"},
	{Database: MariaDB, Cycle: "11.4", Version: "11.4.12", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "11.4"},
	{Database: MariaDB, Cycle: "11.8", Version: "11.8.8", DockerRepo: "library/mariadb", EOLProduct: "mariadb", EOLCycle: "11.8"},
	{Database: TiDB, Cycle: "6.1", Version: "6.1.7", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Database: TiDB, Cycle: "6.5", Version: "6.5.12", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Database: TiDB, Cycle: "7.1", Version: "7.1.6", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Database: TiDB, Cycle: "7.5", Version: "7.5.7", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Database: TiDB, Cycle: "8.1", Version: "8.1.2", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
	{Database: TiDB, Cycle: "8.5", Version: "8.5.6", DockerRepo: "pingcap/tidb", TagPrefix: "v"},
}

type GitHubActionsMatrix struct {
	Include []GitHubActionsEntry `json:"include"`
}

type GitHubActionsEntry struct {
	DBType    string `json:"db_type"`
	DBVersion string `json:"db_version"`
}

func All() []Entry {
	entries := make([]Entry, len(Entries))
	copy(entries, Entries)
	return entries
}

func ActionsMatrix() GitHubActionsMatrix {
	entries := All()
	matrix := GitHubActionsMatrix{Include: make([]GitHubActionsEntry, 0, len(entries))}
	for _, entry := range entries {
		matrix.Include = append(matrix.Include, GitHubActionsEntry{
			DBType:    entry.Database.CLIName(),
			DBVersion: entry.Version,
		})
	}
	return matrix
}

func (entry Entry) DisplayImage() string {
	if entry.Database == TiDB {
		return entry.Version
	}
	return entry.DockerImage()
}

func (entry Entry) DockerImage() string {
	switch entry.Database {
	case MySQL:
		return ImageMySQLPrefix + entry.Version
	case Percona:
		if UsesPerconaServerImage(entry.Version) {
			return ImagePerconaServerPrefix + entry.Version
		}
		return ImagePerconaPrefix + entry.Version
	case MariaDB:
		return ImageMariaDBPrefix + entry.Version
	case TiDB:
		return ImageTiDBPrefix + entry.Version
	default:
		return entry.Database.CLIName() + ":" + entry.Version
	}
}

func ImageForDBVersion(dbType, version string) (string, error) {
	switch strings.ToLower(dbType) {
	case MySQL.CLIName():
		return ImageMySQLPrefix + version, nil
	case Percona.CLIName():
		if UsesPerconaServerImage(version) {
			return ImagePerconaServerPrefix + version, nil
		}
		return ImagePerconaPrefix + version, nil
	case MariaDB.CLIName():
		return ImageMariaDBPrefix + version, nil
	case TiDB.CLIName():
		return ImageTiDBPrefix + version, nil
	default:
		return "", fmt.Errorf("unsupported database type %q", dbType)
	}
}

func NormalizeImageAlias(image string) string {
	if strings.HasPrefix(image, ImagePerconaPrefix) {
		version := strings.TrimPrefix(image, ImagePerconaPrefix)
		if UsesPerconaServerImage(version) {
			return ImagePerconaServerPrefix + version
		}
	}
	return image
}

func InferDatabaseAndDisplayImage(image string) (Database, string, error) {
	switch {
	case strings.HasPrefix(image, ImageMySQLPrefix):
		return MySQL, image, nil
	case strings.HasPrefix(image, ImagePerconaPrefix) || strings.HasPrefix(image, ImagePerconaRepoPrefix) || strings.HasPrefix(image, ImageDockerPerconaRepoPrefix):
		return Percona, image, nil
	case strings.HasPrefix(image, ImageMariaDBPrefix):
		return MariaDB, image, nil
	case strings.HasPrefix(image, ImageTiDBPrefix):
		return TiDB, strings.TrimPrefix(image, ImageTiDBPrefix), nil
	default:
		return "", "", fmt.Errorf("could not infer database type from image %q", image)
	}
}

func IsVersionInSeries(version, series string) bool {
	return version == series || strings.HasPrefix(version, series+".")
}

func UsesPerconaServerImage(version string) bool {
	return IsVersionInSeries(version, VersionPercona80) || IsVersionInSeries(version, VersionPercona84)
}

func ImageIsInSeries(image, prefix, series string) bool {
	if !strings.HasPrefix(image, prefix) {
		return false
	}
	return IsVersionInSeries(strings.TrimPrefix(image, prefix), series)
}

func (db Database) CLIName() string {
	return string(db)
}

func (db Database) Label() string {
	switch db {
	case MySQL:
		return "MySQL"
	case Percona:
		return "Percona"
	case MariaDB:
		return "MariaDB"
	case TiDB:
		return "TiDB"
	default:
		return string(db)
	}
}

func (db Database) ConstName() string {
	switch db {
	case MySQL:
		return "MySQL"
	case Percona:
		return "Percona"
	case MariaDB:
		return "MariaDB"
	case TiDB:
		return "TiDB"
	default:
		return string(db)
	}
}

func (db Database) String() string {
	return db.Label()
}
