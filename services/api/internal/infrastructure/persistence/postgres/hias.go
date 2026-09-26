package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-hia/services/api/internal/domain/hia"
)

// HiaRepository persists hias (Hia module table only).
type HiaRepository struct{ pool *pgxpool.Pool }

func NewHiaRepository(pool *pgxpool.Pool) *HiaRepository { return &HiaRepository{pool: pool} }

const hiaColumns = `id::text, public_number, public_code, nickname, status, home_park_id::text,
	first_seen_at, confirmed_at, merged_into_hia_id::text, created_at, updated_at`

// Create inserts h. PostgreSQL allocates public_number from an identity
// sequence and derives public_code from it, so concurrent inserts can never
// receive the same code. Any PublicCode set on h is ignored.
func (r *HiaRepository) Create(ctx context.Context, h hia.Hia) (hia.Hia, error) {
	if err := h.Validate(); err != nil {
		return hia.Hia{}, err
	}
	created, err := scanHia(r.pool.QueryRow(ctx, `INSERT INTO hias
		(id, nickname, status, home_park_id, first_seen_at, confirmed_at, merged_into_hia_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+hiaColumns,
		h.ID, h.Nickname, h.Status, h.HomeParkID, h.FirstSeenAt, h.ConfirmedAt, h.MergedIntoHiaID, h.CreatedAt, h.UpdatedAt))
	return created, mapHiaError(err)
}

func (r *HiaRepository) List(ctx context.Context, homeParkID *string) ([]hia.Hia, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+hiaColumns+` FROM hias
		WHERE ($1::uuid IS NULL OR home_park_id = $1::uuid) ORDER BY public_number`, homeParkID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (hia.Hia, error) { return scanHia(row) })
}

func (r *HiaRepository) FindByPublicCode(ctx context.Context, code string) (hia.Hia, error) {
	h, err := scanHia(r.pool.QueryRow(ctx, `SELECT `+hiaColumns+` FROM hias WHERE public_code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return hia.Hia{}, hia.ErrNotFound
	}
	return h, err
}

func (r *HiaRepository) PublicCodes(ctx context.Context, ids []string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, public_code FROM hias WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, code string
		if err = rows.Scan(&id, &code); err != nil {
			return nil, err
		}
		out[id] = code
	}
	return out, rows.Err()
}

func scanHia(row pgx.Row) (hia.Hia, error) {
	var h hia.Hia
	err := row.Scan(&h.ID, &h.PublicNumber, &h.PublicCode, &h.Nickname, &h.Status, &h.HomeParkID,
		&h.FirstSeenAt, &h.ConfirmedAt, &h.MergedIntoHiaID, &h.CreatedAt, &h.UpdatedAt)
	return h, err
}

func mapHiaError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "hias_not_merged_into_self":
			return hia.ErrMergeIntoSelf
		case "hias_merge_target_matches_status":
			return hia.ErrMergeTargetMissing
		case "hias_status_valid":
			return hia.ErrInvalidStatus
		}
	}
	return err
}
