package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

func querySingleRowStringMap(db *sql.DB, query string, args ...interface{}) (map[string]string, error) {
	return querySingleRowStringMapContext(context.Background(), db, query, args...)
}

func querySingleRowStringMapContext(ctx context.Context, db *sql.DB, query string, args ...interface{}) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]sql.NullString, len(columns))
	scanValues := make([]interface{}, len(columns))
	for i := range values {
		scanValues[i] = &values[i]
	}

	if err := rows.Scan(scanValues...); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[string]string, len(columns))
	for i, column := range columns {
		if values[i].Valid {
			out[strings.ToUpper(column)] = values[i].String
		} else {
			out[strings.ToUpper(column)] = ""
		}
	}

	if rows.Next() {
		return nil, errors.New("expected one row, got multiple rows")
	}

	return out, nil
}

func stringMapValue(row map[string]string, key string) string {
	return row[strings.ToUpper(key)]
}

func stringMapIntValue(row map[string]string, key string) (int, error) {
	value := strings.TrimSpace(stringMapValue(row, key))
	if value == "" {
		return 0, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}

	return parsed, nil
}
