package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("store: user not found")
	ErrEmailConflict = errors.New("store: email already linked to another identity")
)

type User struct {
	ID              uuid.UUID `json:"id"`
	Email           string    `json:"email"`
	Name            *string   `json:"name"`
	Provider        string    `json:"provider"`
	ProviderUserID  string    `json:"-"`
	AccountantEmail *string   `json:"accountantEmail"`
	CreatedAt       time.Time `json:"createdAt"`
}

type UserRepository interface {
	UpsertByProviderIdentity(ctx context.Context, identity Identity) (User, error)
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
}

type Identity struct {
	Provider       string
	ProviderUserID string
	Email          string
	Name           string
}
