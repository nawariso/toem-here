package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-hia/services/api/internal/domain/park"
)

// ParkRepository persists parks and zones (Park module tables only).
type ParkRepository struct{ pool *pgxpool.Pool }

func NewParkRepository(pool *pgxpool.Pool) *ParkRepository { return &ParkRepository{pool: pool} }

const parkColumns = `id::text, slug, name, city, country_code, timezone, status, created_at, updated_at`
const zoneColumns = `id::text, park_id::text, slug, name, status, created_at, updated_at`

func (r *ParkRepository) CreatePark(ctx context.Context, p park.Park) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO parks (id, slug, name, city, country_code, timezone, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.Slug, p.Name, p.City, p.CountryCode, p.Timezone, p.Status, p.CreatedAt, p.UpdatedAt)
	return mapParkError(err)
}

func (r *ParkRepository) CreateZone(ctx context.Context, z park.Zone) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO zones (id, park_id, slug, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		z.ID, z.ParkID, z.Slug, z.Name, z.Status, z.CreatedAt, z.UpdatedAt)
	return mapParkError(err)
}

// EnsurePark inserts p unless a park with the same slug (case-insensitive)
// exists, and returns the stored park. Existing rows are never modified.
func (r *ParkRepository) EnsurePark(ctx context.Context, p park.Park) (park.Park, bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO parks (id, slug, name, city, country_code, timezone, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`,
		p.ID, p.Slug, p.Name, p.City, p.CountryCode, p.Timezone, p.Status, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return park.Park{}, false, mapParkError(err)
	}
	stored, err := scanPark(r.pool.QueryRow(ctx, `SELECT `+parkColumns+` FROM parks WHERE lower(slug) = lower($1)`, p.Slug))
	return stored, tag.RowsAffected() == 1, err
}

// EnsureZone inserts z unless its park already has a zone with the same slug,
// and returns the stored zone. Existing rows are never modified.
func (r *ParkRepository) EnsureZone(ctx context.Context, z park.Zone) (park.Zone, bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO zones (id, park_id, slug, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		z.ID, z.ParkID, z.Slug, z.Name, z.Status, z.CreatedAt, z.UpdatedAt)
	if err != nil {
		return park.Zone{}, false, mapParkError(err)
	}
	stored, err := scanZone(r.pool.QueryRow(ctx, `SELECT `+zoneColumns+` FROM zones WHERE park_id = $1 AND lower(slug) = lower($2)`, z.ParkID, z.Slug))
	return stored, tag.RowsAffected() == 1, err
}

func (r *ParkRepository) ListActiveParks(ctx context.Context) ([]park.Park, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+parkColumns+` FROM parks WHERE status = 'ACTIVE' ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (park.Park, error) { return scanPark(row) })
}

func (r *ParkRepository) FindActivePark(ctx context.Context, id string) (park.Park, error) {
	p, err := scanPark(r.pool.QueryRow(ctx, `SELECT `+parkColumns+` FROM parks WHERE id = $1 AND status = 'ACTIVE'`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return park.Park{}, park.ErrNotFound
	}
	return p, err
}

func (r *ParkRepository) ListActiveZones(ctx context.Context, parkID string) ([]park.Zone, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+zoneColumns+` FROM zones WHERE park_id = $1 AND status = 'ACTIVE' ORDER BY name, id`, parkID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (park.Zone, error) { return scanZone(row) })
}

func (r *ParkRepository) FindParks(ctx context.Context, ids []string) (map[string]park.Park, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+parkColumns+` FROM parks WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (park.Park, error) { return scanPark(row) })
	if err != nil {
		return nil, err
	}
	out := make(map[string]park.Park, len(list))
	for _, p := range list {
		out[p.ID] = p
	}
	return out, nil
}

func (r *ParkRepository) FindZones(ctx context.Context, ids []string) (map[string]park.Zone, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+zoneColumns+` FROM zones WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (park.Zone, error) { return scanZone(row) })
	if err != nil {
		return nil, err
	}
	out := make(map[string]park.Zone, len(list))
	for _, z := range list {
		out[z.ID] = z
	}
	return out, nil
}

func scanPark(row pgx.Row) (park.Park, error) {
	var p park.Park
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.City, &p.CountryCode, &p.Timezone, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func scanZone(row pgx.Row) (park.Zone, error) {
	var z park.Zone
	err := row.Scan(&z.ID, &z.ParkID, &z.Slug, &z.Name, &z.Status, &z.CreatedAt, &z.UpdatedAt)
	return z, err
}

func mapParkError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "parks_slug_ci_unique":
			return park.ErrSlugTaken
		case "zones_park_slug_ci_unique":
			return park.ErrZoneSlugTaken
		case "zones_park_id_fkey":
			return park.ErrNotFound
		}
	}
	return err
}
