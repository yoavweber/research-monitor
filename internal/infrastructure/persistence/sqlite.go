package persistence

import (
	"fmt"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenSQLite(path string) (*gorm.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dsn := abs + "?_foreign_keys=1&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
		// TranslateError is required so the SQLite driver maps unique
		// constraint violations (errno 1555 / 2067) to gorm.ErrDuplicatedKey;
		// the paper repository's dedupe path depends on this mapping.
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return db, nil
}
