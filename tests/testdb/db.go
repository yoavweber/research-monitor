// Package testdb provides a shared SQLite test database helper. The
// TranslateError flag is set so unique-violation surfaces as
// gorm.ErrDuplicatedKey just like the production helper.
package testdb

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
)

// New opens a fresh SQLite database under t.TempDir() and runs AutoMigrate.
// It is safe for use with t.Parallel(): each call gets its own file under
// the test's temp dir, automatically removed on test cleanup.
func New(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
