package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selim/fatura-backend/internal/store"
)

const uniqueViolation = "23505"

const userColumns = `id, email, name, provider, provider_user_id, accountant_email, created_at`

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) UpsertByProviderIdentity(ctx context.Context, identity store.Identity) (store.User, error) {
	const q = `
		INSERT INTO users (email, name, provider, provider_user_id)
		VALUES ($1, NULLIF($2, ''), $3, $4)
		ON CONFLICT (provider, provider_user_id) DO UPDATE
		SET email  = EXCLUDED.email,
		    name   = COALESCE(EXCLUDED.name, users.name)
		RETURNING ` + userColumns

	user, err := scanUser(r.pool.QueryRow(ctx, q,
		identity.Email,
		identity.Name,
		identity.Provider,
		identity.ProviderUserID,
	))
	if err == nil {
		return user, nil
	}

	if !isUniqueViolation(err) {
		return store.User{}, fmt.Errorf("pgstore: upsert user: %w", err)
	}

	existing, findErr := r.findByEmail(ctx, identity.Email)
	if findErr != nil {
		return store.User{}, fmt.Errorf("pgstore: resolve email conflict: %w", findErr)
	}
	if existing.Provider == identity.Provider && existing.ProviderUserID == identity.ProviderUserID {
		return existing, nil
	}

	return store.User{}, store.ErrEmailConflict
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	user, err := scanUser(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, fmt.Errorf("pgstore: find user by id: %w", err)
	}

	return user, nil
}

func (r *UserRepository) findByEmail(ctx context.Context, email string) (store.User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE email = $1`

	user, err := scanUser(r.pool.QueryRow(ctx, q, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, fmt.Errorf("pgstore: find user by email: %w", err)
	}

	return user, nil
}

func scanUser(row pgx.Row) (store.User, error) {
	var u store.User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Name,
		&u.Provider,
		&u.ProviderUserID,
		&u.AccountantEmail,
		&u.CreatedAt,
	)
	return u, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
