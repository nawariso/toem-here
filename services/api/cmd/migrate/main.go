package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/persistence/postgres/migrations"
)

const usage = "usage: migrate up | down | down-to <version>"

func main() {
	target, down, ok := parseArgs(os.Args[1:])
	if !ok {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database configuration is invalid")
		os.Exit(1)
	}
	defer pool.Close()
	if down {
		err = migrations.DownTo(ctx, pool, target)
	} else {
		err = migrations.UpTo(ctx, pool, target)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration failed")
		os.Exit(1)
	}
	fmt.Println("migration complete; schema version", target)
}

// parseArgs maps the command line to a target version. `down` reverts every
// migration; `down-to 1` reverts only migrations newer than version 1.
func parseArgs(args []string) (target int, down bool, ok bool) {
	switch {
	case len(args) == 1 && args[0] == "up":
		return migrations.Latest(), false, true
	case len(args) == 1 && args[0] == "down":
		return 0, true, true
	case len(args) == 2 && args[0] == "down-to":
		v, err := strconv.Atoi(args[1])
		if err != nil || v < 0 || v > migrations.Latest() {
			return 0, false, false
		}
		return v, true, true
	}
	return 0, false, false
}
