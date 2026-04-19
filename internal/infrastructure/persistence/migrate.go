package persistence

import (
	"fmt"

	"gorm.io/gorm"

	sourcemodel "github.com/yoavweber/defi-monitor-backend/internal/infrastructure/persistence/source"
)

// AutoMigrate runs GORM AutoMigrate over every persistence model in the repo.
// When a new aggregate is added, append its GORM model to this list.
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&sourcemodel.Source{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
