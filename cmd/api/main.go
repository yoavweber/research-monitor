package main

import (
	"context"
	"log"

	"github.com/yoavweber/research-monitor/backend/internal/bootstrap"
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
