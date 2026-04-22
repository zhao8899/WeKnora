package utils

import (
	"strings"
	"unicode"
)

var simpleSQLTailKeywords = []string{" where ", " group by ", " order by ", " having ", " limit ", " offset ", " fetch ", " union "}

func parseSQLFallback(sql string) (*SQLParseResult, bool) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return &SQLParseResult{
			OriginalSQL: sql,
			ParseError:  "empty query",
		}, true
	}

	result := &SQLParseResult{
		OriginalSQL:  sql,
		TableNames:   make([]string, 0),
		SelectFields: make([]string, 0),
		WhereFields:  make([]string, 0),
	}

	statement := strings.TrimSuffix(trimmed, ";")
	lower := strings.ToLower(statement)
	if !strings.HasPrefix(strings.TrimSpace(lower), "select ") {
		return result, true
	}

	fromIdx := findTopLevelKeyword(lower, " from ")
	if fromIdx == -1 {
		result.ParseError = "missing FROM clause"
		return result, true
	}

	result.IsSelect = true
	selectClause := statement[len("select "):fromIdx]
	fromAndTail := statement[fromIdx+len(" from "):]
	tailStart := findFirstTopLevelKeyword(strings.ToLower(fromAndTail), simpleSQLTailKeywords)
	fromClause := fromAndTail
	if tailStart >= 0 {
		fromClause = fromAndTail[:tailStart]
	}

	whereIdx := findTopLevelKeyword(lower, " where ")
	if whereIdx >= 0 {
		whereStart := whereIdx + len(" where ")
		whereTail := statement[whereStart:]
		if stop := findFirstTopLevelKeyword(strings.ToLower(whereTail), simpleSQLTailKeywords[1:]); stop >= 0 {
			whereTail = whereTail[:stop]
		}
		result.WhereClause = strings.TrimSpace(whereTail)
		result.WhereFields = parseWhereFieldsFallback(result.WhereClause)
	}

	result.SelectFields = parseSelectFieldsFallback(selectClause)
	result.TableNames = parseTableNamesFallback(fromClause)
	return result, true
}

func validateSQLFallback(
	sql string, validator *sqlValidator, validationResult *SQLValidationResult,
) (*SQLParseResult, bool) {
	if !validator.canUseFallbackValidation() {
		return nil, false
	}

	result, ok := parseSQLFallback(sql)
	if !ok {
		return nil, false
	}

	if validator.checkSingleStatement && hasMultipleStatements(sql) {
		validationResult.Valid = false
		validationResult.Errors = append(validationResult.Errors, SQLValidationError{
			Type:    "multiple_statements",
			Message: "Multiple statements are not allowed",
			Details: "Fallback parser detected multiple SQL statements",
		})
		return result, true
	}

	if result.ParseError != "" {
		validationResult.Valid = false
		validationResult.Errors = append(validationResult.Errors, SQLValidationError{
			Type:    "parse_error",
			Message: "Failed to parse SQL",
			Details: "SQL parse error: " + result.ParseError,
		})
		return result, true
	}

	if validator.checkSelectOnly && !result.IsSelect {
		validationResult.Valid = false
		validationResult.Errors = append(validationResult.Errors, SQLValidationError{
			Type:    "not_select_statement",
			Message: "Only SELECT queries are allowed",
			Details: "Statement is not a SELECT query",
		})
	}

	if validator.checkTableNames {
		for _, table := range result.TableNames {
			if !validator.allowedTables[strings.ToLower(table)] {
				validationResult.Valid = false
				validationResult.Errors = append(validationResult.Errors, SQLValidationError{
					Type:    "table_not_allowed",
					Message: "Table '" + table + "' is not in the allowed list",
					Details: "Allowed tables: " + strings.Join(getMapKeys(validator.allowedTables), ", "),
				})
			}
		}
	}

	if validator.checkInjectionRisk {
		injectionErrors := checkSQLInjectionRisks(result.WhereClause)
		if len(injectionErrors) > 0 {
			validationResult.Valid = false
			validationResult.Errors = append(validationResult.Errors, injectionErrors...)
		}
	}

	return result, true
}

func (v *sqlValidator) canUseFallbackValidation() bool {
	return !v.checkFunctionNames &&
		!v.checkSubqueries &&
		!v.checkCTEs &&
		!v.checkSystemColumns &&
		!v.checkSchemaAccess &&
		!v.checkDangerousFuncs &&
		!v.enableTenantInjection &&
		!v.enableSoftDeleteInjection &&
		!v.enableHiddenKBFilter &&
		!v.enableSearchScopeFilter
}

func parseSelectFieldsFallback(selectClause string) []string {
	if strings.Contains(selectClause, "*") {
		return []string{}
	}
	items := splitTopLevelCSV(selectClause)
	fields := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		for _, field := range extractColumnTokens(item) {
			if !seen[field] {
				seen[field] = true
				fields = append(fields, field)
			}
		}
	}
	return fields
}

func parseTableNamesFallback(fromClause string) []string {
	tables := []string{}
	seen := map[string]bool{}
	tokens := strings.Fields(strings.NewReplacer(",", " ", "(", " ", ")", " ").Replace(fromClause))
	expectTable := true
	for i := 0; i < len(tokens); i++ {
		token := stripIdentifier(tokens[i])
		lower := strings.ToLower(token)
		if lower == "" {
			continue
		}
		switch lower {
		case "join":
			expectTable = true
			continue
		case "inner", "left", "right", "full", "outer", "cross", "on", "as":
			continue
		}
		if expectTable {
			expectTable = false
			if !seen[lower] {
				seen[lower] = true
				tables = append(tables, lower)
			}
		}
	}
	return tables
}

func parseWhereFieldsFallback(whereClause string) []string {
	if whereClause == "" {
		return nil
	}
	fields := []string{}
	seen := map[string]bool{}
	for _, token := range strings.FieldsFunc(whereClause, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.')
	}) {
		if token == "" {
			continue
		}
		last := token
		if dot := strings.LastIndex(last, "."); dot >= 0 {
			last = last[dot+1:]
		}
		lower := strings.ToLower(stripIdentifier(last))
		if lower == "" || isSQLKeyword(lower) || len(lower) > 0 && unicode.IsDigit(rune(lower[0])) {
			continue
		}
		if !strings.Contains(strings.ToLower(whereClause), token) {
			continue
		}
		if !seen[lower] {
			seen[lower] = true
			fields = append(fields, lower)
		}
	}
	return filterWhereFieldsByOperatorOrder(whereClause, fields)
}

func filterWhereFieldsByOperatorOrder(whereClause string, candidates []string) []string {
	result := []string{}
	seen := map[string]bool{}
	lowerWhere := strings.ToLower(whereClause)
	for _, field := range candidates {
		if seen[field] {
			continue
		}
		if strings.Contains(lowerWhere, field+" =") ||
			strings.Contains(lowerWhere, field+">") ||
			strings.Contains(lowerWhere, field+" >") ||
			strings.Contains(lowerWhere, field+"<") ||
			strings.Contains(lowerWhere, field+" <") ||
			strings.Contains(lowerWhere, field+" between ") ||
			strings.Contains(lowerWhere, field+" in ") ||
			strings.Contains(lowerWhere, field+" like ") ||
			strings.Contains(lowerWhere, field+" is ") {
			seen[field] = true
			result = append(result, field)
		}
	}
	return result
}

func extractColumnTokens(expr string) []string {
	result := []string{}
	seen := map[string]bool{}
	runes := []rune(expr)
	for i := 0; i < len(runes); {
		if !(unicode.IsLetter(runes[i]) || runes[i] == '_') {
			i++
			continue
		}
		start := i
		for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '.') {
			i++
		}
		token := string(runes[start:i])
		base := token
		if dot := strings.LastIndex(base, "."); dot >= 0 {
			base = base[dot+1:]
		}
		base = strings.ToLower(stripIdentifier(base))
		j := i
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		if base == "" || isSQLKeyword(base) || (j < len(runes) && runes[j] == '(' && !strings.Contains(token, ".")) {
			continue
		}
		if !seen[base] {
			seen[base] = true
			result = append(result, base)
		}
	}
	return result
}

func splitTopLevelCSV(s string) []string {
	var parts []string
	depth := 0
	inSingle := false
	start := 0
	runes := []rune(s)
	for i, r := range runes {
		switch r {
		case '\'':
			inSingle = !inSingle
		case '(':
			if !inSingle {
				depth++
			}
		case ')':
			if !inSingle && depth > 0 {
				depth--
			}
		case ',':
			if !inSingle && depth == 0 {
				parts = append(parts, strings.TrimSpace(string(runes[start:i])))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(string(runes[start:])))
	return parts
}

func findTopLevelKeyword(s, keyword string) int {
	return findFirstTopLevelKeyword(s, []string{keyword})
}

func findFirstTopLevelKeyword(s string, keywords []string) int {
	depth := 0
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && depth > 0 {
				depth--
			}
		}
		if inSingle || inDouble || depth != 0 {
			continue
		}
		for _, keyword := range keywords {
			if i+len(keyword) <= len(s) && s[i:i+len(keyword)] == keyword {
				return i
			}
		}
	}
	return -1
}

func hasMultipleStatements(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return false
	}
	inSingle := false
	inDouble := false
	seenTerminator := false
	for i := 0; i < len(trimmed); i++ {
		switch trimmed[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ';':
			if !inSingle && !inDouble {
				if seenTerminator {
					return true
				}
				seenTerminator = true
			}
		default:
			if seenTerminator && !unicode.IsSpace(rune(trimmed[i])) {
				return true
			}
		}
	}
	return false
}

func stripIdentifier(s string) string {
	return strings.Trim(s, "`\"[]")
}

func isSQLKeyword(s string) bool {
	switch s {
	case "select", "from", "where", "join", "inner", "left", "right", "full", "outer",
		"on", "and", "or", "not", "between", "in", "like", "ilike", "is", "null",
		"true", "false", "as", "case", "when", "then", "else", "end", "distinct":
		return true
	default:
		return false
	}
}
