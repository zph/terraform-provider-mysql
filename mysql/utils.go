package mysql

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"github.com/hashicorp/go-version"
)

func hashSum(contents interface{}) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(contents.(string))))
}

func getDatabaseFromMeta(meta interface{}) *sql.DB {
	return meta.(*MySQLConfiguration).Db
}

func getVersionFromMeta(meta interface{}) *version.Version {
	return meta.(*MySQLConfiguration).Version
}
