package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain/encounter"
	"github.com/nawariso/toem-here/services/api/internal/domain/location"
)

// EncounterRepository persists encounters and their private locations
// (Encounter module tables only). Reads of the encounter never select the
// private location, so coordinates cannot reach a response by accident.
type EncounterRepository struct{ pool *pgxpool.Pool }

func NewEncounterRepository(pool *pgxpool.Pool) *EncounterRepository {
	return &EncounterRepository{pool: pool}
}

const encounterColumns = `id::text, observer_user_id::text, captured_at, submitted_at, park_id::text, zone_id::text,
	status, behavior, notes, created_at, updated_at`

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (r *EncounterRepository) Create(ctx context.Context, e encounter.Encounter, loc *location.Private) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO encounters
		(id, observer_user_id, captured_at, submitted_at, park_id, zone_id, status, behavior, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		e.ID, e.ObserverUserID, e.CapturedAt, e.SubmittedAt, e.ParkID, e.ZoneID, e.Status, e.Behavior, e.Notes, e.CreatedAt, e.UpdatedAt); err != nil {
		return mapEncounterError(err)
	}
	if loc != nil {
		if err = putLocation(ctx, tx, e.ID, *loc); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *EncounterRepository) FindByID(ctx context.Context, id string) (encounter.Encounter, error) {
	e, err := scanEncounter(r.pool.QueryRow(ctx, `SELECT `+encounterColumns+` FROM encounters WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return encounter.Encounter{}, encounter.ErrNotFound
	}
	return e, err
}

func (r *EncounterRepository) ListByObserver(ctx context.Context, observerUserID string) ([]encounter.Encounter, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+encounterColumns+` FROM encounters
		WHERE observer_user_id = $1 ORDER BY captured_at DESC, id`, observerUserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (encounter.Encounter, error) { return scanEncounter(row) })
}

// Update serialises concurrent edits/submits of one encounter with a row
// lock, so two submits cannot both observe DRAFT.
func (r *EncounterRepository) Update(ctx context.Context, id string, mutate func(*encounter.Encounter) (application.LocationChange, error)) (encounter.Encounter, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return encounter.Encounter{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	e, err := scanEncounter(tx.QueryRow(ctx, `SELECT `+encounterColumns+` FROM encounters WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return encounter.Encounter{}, encounter.ErrNotFound
	}
	if err != nil {
		return encounter.Encounter{}, err
	}
	change, err := mutate(&e)
	if err != nil {
		return encounter.Encounter{}, err
	}
	// observer_user_id, id, and created_at are deliberately not updatable.
	if _, err = tx.Exec(ctx, `UPDATE encounters SET captured_at = $2, submitted_at = $3, park_id = $4, zone_id = $5,
		status = $6, behavior = $7, notes = $8, updated_at = $9 WHERE id = $1`,
		e.ID, e.CapturedAt, e.SubmittedAt, e.ParkID, e.ZoneID, e.Status, e.Behavior, e.Notes, e.UpdatedAt); err != nil {
		return encounter.Encounter{}, mapEncounterError(err)
	}
	if change.Set {
		if change.Value == nil {
			if _, err = tx.Exec(ctx, `DELETE FROM encounter_locations WHERE encounter_id = $1`, e.ID); err != nil {
				return encounter.Encounter{}, err
			}
		} else if err = putLocation(ctx, tx, e.ID, *change.Value); err != nil {
			return encounter.Encounter{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return encounter.Encounter{}, err
	}
	return e, nil
}

// putLocation stores a WGS 84 point as geography(Point, 4326) through
// encounter_location_point, which rejects out-of-range input that a plain
// geography cast would silently coerce into range.
func putLocation(ctx context.Context, tx execer, encounterID string, loc location.Private) error {
	if err := loc.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO encounter_locations (encounter_id, point, accuracy_meters, source)
		VALUES ($1, encounter_location_point($2, $3), $4, $5)
		ON CONFLICT (encounter_id) DO UPDATE
		SET point = EXCLUDED.point, accuracy_meters = EXCLUDED.accuracy_meters, source = EXCLUDED.source, created_at = now()`,
		encounterID, loc.Longitude, loc.Latitude, loc.AccuracyMeters, loc.Source)
	return err
}

func scanEncounter(row pgx.Row) (encounter.Encounter, error) {
	var e encounter.Encounter
	err := row.Scan(&e.ID, &e.ObserverUserID, &e.CapturedAt, &e.SubmittedAt, &e.ParkID, &e.ZoneID,
		&e.Status, &e.Behavior, &e.Notes, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func mapEncounterError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "encounters_zone_in_park":
			return encounter.ErrZoneNotInPark
		case "encounters_park_id_fkey":
			return encounter.ErrParkNotFound
		case "encounters_behavior_valid":
			return encounter.ErrInvalidBehavior
		case "encounters_zone_requires_park":
			return encounter.ErrZoneRequiresPark
		}
	}
	return err
}
