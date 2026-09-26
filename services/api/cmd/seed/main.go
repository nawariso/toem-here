// Command seed loads development reference data (Lumpini Park and its zones).
// It refuses to run unless APP_ENV is development or test.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/nawariso/toem-hia/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/seed"
)

func main() {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if !seed.Permitted(appEnv) {
		fmt.Fprintln(os.Stderr, seed.ErrForbiddenEnvironment.Error())
		os.Exit(1)
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
	result, err := seed.Run(ctx, appEnv, persistence.NewParkRepository(pool), time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
	fmt.Printf("seed complete: park created=%t, zones created=%d, zones already present=%d\n",
		result.ParkCreated, result.ZonesCreated, result.ZonesExisting)
}
