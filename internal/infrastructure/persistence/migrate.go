package persistence

import (
	"fmt"

	"gorm.io/gorm"
)

// AutoMigrate runs GORM AutoMigrate over every persistence model in the repo.
// When a new aggregate is added, append its GORM model to this list.
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
