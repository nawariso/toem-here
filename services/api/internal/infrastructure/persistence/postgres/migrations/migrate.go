package migrations

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 000001_identity.up.sql
var upSQL string

//go:embed 000001_identity.down.sql
var downSQL string

func Up(ctx context.Context, pool *pgxpool.Pool) error { _, err := pool.Exec(ctx, upSQL); return err }
func Down(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, downSQL)
	return err
}
