package auth

import (
	"context"
	"errors"
)

var (
	ErrInvalidToken        = errors.New("auth: invalid provider token")
	ErrEmailNotVerified    = errors.New("auth: provider email is not verified")
	ErrUnsupportedProvider = errors.New("auth: unsupported provider")
)

type Identity struct {
	Provider       string
	ProviderUserID string
	Email          string
	EmailVerified  bool
	Name           string
	Nonce          string
}

type Verifier interface {
	Provider() string
	Verify(ctx context.Context, rawToken string) (Identity, error)
}
