//go:build !windows

package container

import (
	"database/sql"

	_ "github.com/duckdb/duckdb-go/v2"
)

func openDuckDB(dsn string) (*sql.DB, error) {
	return sql.Open("duckdb", dsn)
}
