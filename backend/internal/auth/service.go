package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/selim/fatura-backend/internal/store"
)

type SignInRequest struct {
	Token string
	Name  string
}

type Session struct {
	Token     string     `json:"token"`
	TokenType string     `json:"tokenType"`
	ExpiresAt time.Time  `json:"expiresAt"`
	User      store.User `json:"user"`
}

type Service struct {
	repo      store.UserRepository
	issuer    *TokenIssuer
	verifiers map[string]Verifier
}

func NewService(repo store.UserRepository, issuer *TokenIssuer, verifiers ...Verifier) (*Service, error) {
	if len(verifiers) == 0 {
		return nil, errors.New("auth: at least one verifier is required")
	}

	registry := make(map[string]Verifier, len(verifiers))
	for _, v := range verifiers {
		registry[strings.ToLower(v.Provider())] = v
	}

	return &Service{repo: repo, issuer: issuer, verifiers: registry}, nil
}

func (s *Service) SignIn(ctx context.Context, provider string, req SignInRequest) (Session, error) {
	verifier, ok := s.verifiers[strings.ToLower(strings.TrimSpace(provider))]
	if !ok {
		return Session{}, ErrUnsupportedProvider
	}

	identity, err := verifier.Verify(ctx, req.Token)
	if err != nil {
		return Session{}, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = identity.Name
	}

	user, err := s.repo.UpsertByProviderIdentity(ctx, store.Identity{
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
		Email:          identity.Email,
		Name:           name,
	})
	if err != nil {
		return Session{}, fmt.Errorf("auth: persist identity: %w", err)
	}

	token, expiresAt, err := s.issuer.Issue(user)
	if err != nil {
		return Session{}, err
	}

	return Session{Token: token, TokenType: "Bearer", ExpiresAt: expiresAt, User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, claims *SessionClaims) (store.User, error) {
	return s.repo.FindByID(ctx, claims.UserID)
}
