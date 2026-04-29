// @title           Research Monitor API
// @version         1.0
// @description     HTTP API for the research monitor (arXiv ingestion + paper catalogue).
// @BasePath        /api
// @securityDefinitions.apikey APIToken
// @in              header
// @name            Authorization
package main

import (
	"context"
	"log"

	"github.com/yoavweber/research-monitor/backend/internal/bootstrap"

	_ "github.com/yoavweber/research-monitor/backend/docs"
)

func main() {
	ctx := context.Background()

	env, err := bootstrap.LoadEnv()
	if err != nil {
		log.Fatalf("load env: %v", err)
	}

	app, err := bootstrap.NewApp(ctx, env)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	if err := app.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
