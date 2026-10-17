package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/store"
)

var ErrSessionInvalid = errors.New("auth: session token is invalid")

const sessionIssuer = "fatura-api"

type SessionClaims struct {
	UserID   uuid.UUID `json:"uid"`
	Email    string    `json:"email"`
	Name     string    `json:"name,omitempty"`
	Provider string    `json:"provider"`
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenIssuer(secret []byte, ttl time.Duration) (*TokenIssuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: signing secret must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("auth: token ttl must be positive")
	}

	return &TokenIssuer{secret: secret, ttl: ttl, now: time.Now}, nil
}

func (t *TokenIssuer) Issue(user store.User) (string, time.Time, error) {
	now := t.now().UTC()
	expiresAt := now.Add(t.ttl)

	claims := SessionClaims{
		UserID:   user.ID,
		Email:    user.Email,
		Provider: user.Provider,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sessionIssuer,
			Subject:   user.ID.String(),
			ID:        uuid.NewString(),
			Audience:  jwt.ClaimStrings{"fatura-mobile"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	if user.Name != nil {
		claims.Name = *user.Name
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign session token: %w", err)
	}

	return signed, expiresAt, nil
}

func (t *TokenIssuer) Parse(rawToken string) (*SessionClaims, error) {
	claims := &SessionClaims{}

	parsed, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method %v", ErrSessionInvalid, token.Header["alg"])
			}
			return t.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(sessionIssuer),
		jwt.WithAudience("fatura-mobile"),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionInvalid, err)
	}
	if !parsed.Valid || claims.UserID == uuid.Nil {
		return nil, ErrSessionInvalid
	}

	return claims, nil
}
