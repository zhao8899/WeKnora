package database

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"gorm.io/gorm"
)

type SchemaIssueSeverity string

const (
	SchemaIssueSeverityCritical SchemaIssueSeverity = "critical"
	SchemaIssueSeverityWarning  SchemaIssueSeverity = "warning"
)

// SchemaIssue describes one schema integrity problem discovered during startup audit.
type SchemaIssue struct {
	Severity SchemaIssueSeverity
	Table    string
	Check    string
	Message  string
}

// SchemaAuditReport captures the startup audit result for the live database schema.
type SchemaAuditReport struct {
	Version uint
	Dirty   bool
	Issues  []SchemaIssue
}

var (
	schemaAuditMu      sync.RWMutex
	currentSchemaAudit SchemaAuditReport
	schemaAuditSet     bool
)

// CachedSchemaAuditReport returns the last schema audit captured at startup.
func CachedSchemaAuditReport() (SchemaAuditReport, bool) {
	schemaAuditMu.RLock()
	defer schemaAuditMu.RUnlock()
	return currentSchemaAudit, schemaAuditSet
}

func setSchemaAuditReport(report SchemaAuditReport) {
	schemaAuditMu.Lock()
	defer schemaAuditMu.Unlock()
	currentSchemaAudit = report
	schemaAuditSet = true
}

func (r SchemaAuditReport) HasCritical() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SchemaIssueSeverityCritical {
			return true
		}
	}
	return false
}

func (r SchemaAuditReport) WarningCount() int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == SchemaIssueSeverityWarning {
			count++
		}
	}
	return count
}

func (r SchemaAuditReport) CriticalCount() int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == SchemaIssueSeverityCritical {
			count++
		}
	}
	return count
}

func (r SchemaAuditReport) Summary() string {
	if len(r.Issues) == 0 {
		return "no schema issues detected"
	}

	parts := make([]string, 0, len(r.Issues))
	for _, issue := range r.Issues {
		parts = append(parts, fmt.Sprintf("%s:%s.%s", issue.Severity, issue.Table, issue.Check))
	}
	return strings.Join(parts, ", ")
}

type migrationVersionRow struct {
	Version uint
	Dirty   bool
}

func loadSchemaMigrationVersion(db *gorm.DB) (uint, bool, bool) {
	if version, dirty, ok := CachedMigrationVersion(); ok {
		return version, dirty, true
	}
	if !db.Migrator().HasTable("schema_migrations") {
		return 0, false, false
	}

	var row migrationVersionRow
	if err := db.Table("schema_migrations").Select("version", "dirty").Limit(1).Scan(&row).Error; err != nil {
		return 0, false, false
	}

	setMigrationVersion(row.Version, row.Dirty)
	return row.Version, row.Dirty, true
}

func hasTable(db *gorm.DB, table string) bool {
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`,
		table,
	).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func hasColumn(db *gorm.DB, table, column string) bool {
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		table,
		column,
	).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func hasIndex(db *gorm.DB, table, index string) bool {
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ? AND indexname = ?`,
		table,
		index,
	).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// RepairKnownSchemaDrift repairs a narrow set of additive schema drifts that have been
// observed in historical deployments. The repairs are idempotent and safe to run on every startup.
func RepairKnownSchemaDrift(db *gorm.DB) error {
	ctx := context.Background()

	type repair struct {
		table       string
		column      string
		ddl         string
		indexName   string
		indexDDL    string
		description string
	}

	repairs := []repair{
		{
			table:       "models",
			column:      "is_platform",
			ddl:         "ALTER TABLE models ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE",
			indexName:   "idx_models_is_platform",
			indexDDL:    "CREATE INDEX IF NOT EXISTS idx_models_is_platform ON models(is_platform) WHERE is_platform = true",
			description: "platform-shared models",
		},
		{
			table:       "web_search_providers",
			column:      "is_platform",
			ddl:         "ALTER TABLE web_search_providers ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE",
			indexName:   "idx_web_search_providers_is_platform",
			indexDDL:    "CREATE INDEX IF NOT EXISTS idx_web_search_providers_is_platform ON web_search_providers(is_platform) WHERE is_platform = true",
			description: "platform-shared web search providers",
		},
		{
			table:       "mcp_services",
			column:      "is_platform",
			ddl:         "ALTER TABLE mcp_services ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE",
			indexName:   "idx_mcp_services_is_platform",
			indexDDL:    "CREATE INDEX IF NOT EXISTS idx_mcp_services_is_platform ON mcp_services(is_platform) WHERE is_platform = true",
			description: "platform-shared MCP services",
		},
	}

	for _, item := range repairs {
		if !hasTable(db, item.table) {
			continue
		}

		columnExists := hasColumn(db, item.table, item.column)
		indexExists := item.indexName == "" || hasIndex(db, item.table, item.indexName)
		if columnExists && indexExists {
			continue
		}

		logger.Warnf(
			ctx,
			"[Database] schema drift detected: %s.%s or %s missing; repairing %s",
			item.table, item.column, item.indexName, item.description,
		)

		if err := db.Exec(item.ddl).Error; err != nil {
			return fmt.Errorf("repair %s.%s failed: %w", item.table, item.column, err)
		}
		if item.indexDDL != "" {
			if err := db.Exec(item.indexDDL).Error; err != nil {
				return fmt.Errorf("repair index %s failed: %w", item.indexName, err)
			}
		}

		logger.Infof(ctx, "[Database] repaired schema drift for %s", item.description)
	}

	if hasTable(db, "models") && hasColumn(db, "models", "is_builtin") {
		logger.Warnf(ctx, "[Database] legacy schema detected: models.is_builtin still exists; migrating to is_platform")
		if err := db.Exec(
			"UPDATE models SET is_platform = TRUE WHERE is_builtin = TRUE AND is_platform = FALSE",
		).Error; err != nil {
			return fmt.Errorf("migrate legacy models.is_builtin data failed: %w", err)
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_models_is_builtin").Error; err != nil {
			return fmt.Errorf("drop legacy idx_models_is_builtin failed: %w", err)
		}
		if err := db.Exec("ALTER TABLE models DROP COLUMN IF EXISTS is_builtin").Error; err != nil {
			return fmt.Errorf("drop legacy models.is_builtin failed: %w", err)
		}
		logger.Infof(ctx, "[Database] removed legacy models.is_builtin column after migrating data to is_platform")
	}

	return nil
}

// AuditSchemaIntegrity verifies that the live schema satisfies the minimum shape implied by
// the recorded migration version. Critical issues should block startup.
func AuditSchemaIntegrity(db *gorm.DB) (SchemaAuditReport, error) {
	report := SchemaAuditReport{}
	version, dirty, ok := loadSchemaMigrationVersion(db)
	if ok {
		report.Version = version
		report.Dirty = dirty
	}

	type check struct {
		minVersion uint
		severity   SchemaIssueSeverity
		table      string
		check      string
		message    string
		present    func(*gorm.DB) bool
	}

	checks := []check{
		{
			minVersion: 36,
			severity:   SchemaIssueSeverityCritical,
			table:      "models",
			check:      "is_platform",
			message:    "migration >= 36 requires models.is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "models") && hasColumn(db, "models", "is_platform")
			},
		},
		{
			minVersion: 36,
			severity:   SchemaIssueSeverityCritical,
			table:      "models",
			check:      "idx_models_is_platform",
			message:    "migration >= 36 requires idx_models_is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "models") && hasIndex(db, "models", "idx_models_is_platform")
			},
		},
		{
			minVersion: 41,
			severity:   SchemaIssueSeverityCritical,
			table:      "web_search_providers",
			check:      "is_platform",
			message:    "migration >= 41 requires web_search_providers.is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "web_search_providers") &&
					hasColumn(db, "web_search_providers", "is_platform")
			},
		},
		{
			minVersion: 41,
			severity:   SchemaIssueSeverityCritical,
			table:      "web_search_providers",
			check:      "idx_web_search_providers_is_platform",
			message:    "migration >= 41 requires idx_web_search_providers_is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "web_search_providers") &&
					hasIndex(db, "web_search_providers", "idx_web_search_providers_is_platform")
			},
		},
		{
			minVersion: 41,
			severity:   SchemaIssueSeverityCritical,
			table:      "mcp_services",
			check:      "is_platform",
			message:    "migration >= 41 requires mcp_services.is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "mcp_services") && hasColumn(db, "mcp_services", "is_platform")
			},
		},
		{
			minVersion: 41,
			severity:   SchemaIssueSeverityCritical,
			table:      "mcp_services",
			check:      "idx_mcp_services_is_platform",
			message:    "migration >= 41 requires idx_mcp_services_is_platform",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "mcp_services") && hasIndex(db, "mcp_services", "idx_mcp_services_is_platform")
			},
		},
		{
			minVersion: 40,
			severity:   SchemaIssueSeverityWarning,
			table:      "models",
			check:      "legacy_is_builtin",
			message:    "migration >= 40 should have removed models.is_builtin; legacy column still exists",
			present: func(db *gorm.DB) bool {
				return hasTable(db, "models") && hasColumn(db, "models", "is_builtin")
			},
		},
	}

	for _, item := range checks {
		if ok && version < item.minVersion {
			continue
		}
		if item.present(db) {
			// The legacy column warning is inverted: presence is the issue.
			if item.check == "legacy_is_builtin" {
				report.Issues = append(report.Issues, SchemaIssue{
					Severity: item.severity,
					Table:    item.table,
					Check:    item.check,
					Message:  item.message,
				})
			}
			continue
		}
		if item.check == "legacy_is_builtin" {
			continue
		}

		report.Issues = append(report.Issues, SchemaIssue{
			Severity: item.severity,
			Table:    item.table,
			Check:    item.check,
			Message:  item.message,
		})
	}

	setSchemaAuditReport(report)
	return report, nil
}
