package main

import (
	"context"
	"log"

	"github.com/yoavweber/research-monitor/backend/internal/bootstrap"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
)

func main() {
	ctx := context.Background()

	env, err := bootstrap.LoadEnv()
	if err != nil {
		log.Fatalf("load env: %v", err)
	}

	logger := observability.NewLogger(env.AppEnv)

	db, err := persistence.OpenSQLite(env.SQLitePath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := bootstrap.SeedSources(ctx, db, shared.SystemClock{}, logger); err != nil {
		log.Fatalf("seed: %v", err)
	}
}
