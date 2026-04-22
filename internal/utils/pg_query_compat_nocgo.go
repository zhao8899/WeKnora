//go:build !cgo

package utils

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func parsePGQuerySQL(sql string) (*pg_query.ParseResult, error) {
	return nil, fmt.Errorf("SQL parsing requires CGO-enabled build for github.com/pganalyze/pg_query_go/v6")
}

func deparsePGQuerySQL(tree *pg_query.ParseResult) (string, error) {
	return "", fmt.Errorf("SQL deparsing requires CGO-enabled build for github.com/pganalyze/pg_query_go/v6")
}
