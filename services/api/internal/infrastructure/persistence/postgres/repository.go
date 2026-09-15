package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository   { return &Repository{pool: pool} }
func (r *Repository) Pool() *pgxpool.Pool            { return r.pool }
func (r *Repository) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

func (r *Repository) Bootstrap(ctx context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := identity.Provider + ":" + identity.Subject
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return domain.User{}, err
	}
	user, err := findByIdentity(ctx, tx, identity)
	if err == nil {
		if _, err = tx.Exec(ctx, "UPDATE auth_identities SET last_login_at=now() WHERE provider=$1 AND provider_subject=$2", identity.Provider, identity.Subject); err != nil {
			return domain.User{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.User{}, err
		}
		return user, nil
	}
	if !errors.Is(err, application.ErrNotFound) {
		return domain.User{}, err
	}
	user = domain.NewUser("th")
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,locale,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5)`, user.ID, user.Locale, user.Status, user.CreatedAt, user.UpdatedAt); err != nil {
		return domain.User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_identities(id,user_id,provider,provider_subject) VALUES($1,$2,$3,$4)`, uuid.NewString(), user.ID, identity.Provider, identity.Subject); err != nil {
		return domain.User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role) VALUES($1,$2)`, user.ID, domain.RoleUser); err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	user.Roles = []string{domain.RoleUser}
	return user, nil
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func findByIdentity(ctx context.Context, q querier, identity domain.ExternalIdentity) (domain.User, error) {
	var u domain.User
	err := q.QueryRow(ctx, `SELECT u.id::text,u.username,u.display_name,u.avatar_url,u.locale,u.status,u.created_at,u.updated_at,u.deleted_at FROM users u JOIN auth_identities a ON a.user_id=u.id WHERE a.provider=$1 AND a.provider_subject=$2`, identity.Provider, identity.Subject).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Locale, &u.Status, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, application.ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	rows, err := q.Query(ctx, "SELECT role FROM user_roles WHERE user_id=$1 ORDER BY role", u.ID)
	if err != nil {
		return domain.User{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			return domain.User{}, err
		}
		u.Roles = append(u.Roles, role)
	}
	return u, rows.Err()
}
func (r *Repository) FindByIdentity(ctx context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	return findByIdentity(ctx, r.pool, identity)
}
func (r *Repository) UpdateProfile(ctx context.Context, identity domain.ExternalIdentity, patch domain.ProfilePatch) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	u, err := findByIdentity(ctx, tx, identity)
	if err != nil {
		return domain.User{}, err
	}
	username, displayName, locale := u.Username, u.DisplayName, u.Locale
	if patch.Username != nil {
		username = patch.Username
	}
	if patch.DisplayName != nil {
		displayName = patch.DisplayName
	}
	if patch.Locale != nil {
		locale = *patch.Locale
	}
	_, err = tx.Exec(ctx, "UPDATE users SET username=$1,display_name=$2,locale=$3,updated_at=$4 WHERE id=$5", username, displayName, locale, time.Now().UTC(), u.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "users_username_ci_unique" {
			return domain.User{}, domain.ErrUsernameTaken
		}
		return domain.User{}, err
	}
	updated, err := findByIdentity(ctx, tx, identity)
	if err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return updated, nil
}
