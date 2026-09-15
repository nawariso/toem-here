package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres/migrations"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
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
	if os.Args[1] == "up" {
		err = migrations.Up(ctx, pool)
	} else {
		err = migrations.Down(ctx, pool)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration failed")
		os.Exit(1)
	}
	fmt.Println("migration", os.Args[1], "complete")
}
