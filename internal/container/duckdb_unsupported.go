//go:build windows

package container

import (
	"database/sql"
	"fmt"
)

func openDuckDB(_ string) (*sql.DB, error) {
	return nil, fmt.Errorf("duckdb is not supported in this Windows build environment")
}
