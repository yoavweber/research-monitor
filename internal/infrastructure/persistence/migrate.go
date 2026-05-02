package persistence

import (
	"fmt"

	"gorm.io/gorm"

	analyzerpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/analyzer"
	extractionpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	paperpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/paper"
	sourcemodel "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
)

// AutoMigrate runs GORM AutoMigrate over every persistence model in the repo.
// When a new aggregate is added, append its GORM model to this list.
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&sourcemodel.Source{},
		&paperpersist.Paper{},
		&extractionpersist.Extraction{},
		&analyzerpersist.Analysis{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
