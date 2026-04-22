//go:build cgo
// +build cgo

package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSchemaAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE schema_migrations (version BIGINT NOT NULL, dirty BOOLEAN NOT NULL)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO schema_migrations(version, dirty) VALUES (41, false)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE models (id TEXT PRIMARY KEY, is_platform BOOLEAN NOT NULL DEFAULT FALSE)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_models_is_platform ON models(is_platform)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE web_search_providers (id TEXT PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE mcp_services (id TEXT PRIMARY KEY)`).Error)

	return db
}

func TestAuditSchemaIntegrityDetectsMissingPlatformColumns(t *testing.T) {
	db := newSchemaAuditTestDB(t)

	report, err := AuditSchemaIntegrity(db)
	require.NoError(t, err)
	require.True(t, report.HasCritical())
	require.GreaterOrEqual(t, report.CriticalCount(), 4)
}

func TestAuditSchemaIntegrityPassesAfterExpectedColumnsAndIndexesExist(t *testing.T) {
	db := newSchemaAuditTestDB(t)

	require.NoError(t, db.Exec(`ALTER TABLE web_search_providers ADD COLUMN is_platform BOOLEAN NOT NULL DEFAULT FALSE`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_web_search_providers_is_platform ON web_search_providers(is_platform)`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE mcp_services ADD COLUMN is_platform BOOLEAN NOT NULL DEFAULT FALSE`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_mcp_services_is_platform ON mcp_services(is_platform)`).Error)

	report, err := AuditSchemaIntegrity(db)
	require.NoError(t, err)
	require.False(t, report.HasCritical())
	require.Zero(t, report.CriticalCount())
}
