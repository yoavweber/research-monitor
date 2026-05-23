package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/yoavweber/research-monitor/backend/internal/bootstrap"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/auth"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/observability"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
)

// bcryptProdCost is the production-grade bcrypt work factor. It matches the
// baseline the design picks for user-auth (Req 7.2): expensive enough to slow
// offline cracking, cheap enough that login latency stays in single-digit
// hundreds of milliseconds on the operator host.
const bcryptProdCost = 12

const usage = `usage:
  seed                              seed canonical source rows (idempotent)
  seed users <email> <password>     create or skip a user (idempotent)
  seed -h | --help                  show this message
`

func main() {
	ctx := context.Background()

	// Routing happens before env/DB so `-h` works without a configured
	// environment — an operator running --help on a fresh checkout should
	// not need a .env file to discover the subcommands.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Print(usage)
			return
		case "users":
			runSeedUsers(ctx, os.Args[2:])
			return
		default:
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
	}

	runSeedSources(ctx)
}

// runSeedSources preserves the legacy no-args behavior verbatim. Splitting it
// out keeps the dispatch switch above readable without duplicating wiring.
func runSeedSources(ctx context.Context) {
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

// runSeedUsers parses the `users` subcommand arguments and delegates to
// bootstrap.SeedUser. Errors from the helper are surfaced verbatim to stderr
// — they are already crafted to omit the plaintext password (Req 8.5).
func runSeedUsers(ctx context.Context, args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: seed users <email> <password>")
		os.Exit(2)
	}
	email, password := args[0], args[1]

	env, err := bootstrap.LoadEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load env: %v\n", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(env.AppEnv)

	db, err := persistence.OpenSQLite(env.SQLitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	hasher := auth.NewBcryptHasher(bcryptProdCost)

	if err := bootstrap.SeedUser(ctx, db, hasher, logger, email, password); err != nil {
		fmt.Fprintf(os.Stderr, "seed user: %v\n", err)
		os.Exit(1)
	}
}
