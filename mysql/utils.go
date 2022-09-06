package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"github.com/hashicorp/go-version"
	"log"
)

func hashSum(contents interface{}) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(contents.(string))))
}

func getDatabaseFromMeta(ctx context.Context, meta interface{}) (*sql.DB, error) {
	mysqlConf := meta.(*MySQLConfiguration)
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %v", err)
	}

	return oneConnection.Db, nil
}

func getVersionFromMeta(ctx context.Context, meta interface{}) *version.Version {
	mysqlConf := meta.(*MySQLConfiguration)
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)
	if err != nil {
		log.Panicf("getting DB got us error: %v", err)
	}

	return oneConnection.Version
}
