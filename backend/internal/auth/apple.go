package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/golang-jwt/jwt/v5"
)

const (
	providerApple      = "apple"
	defaultAppleJWKS   = "https://appleid.apple.com/auth/keys"
	defaultAppleIssuer = "https://appleid.apple.com"
)

type AppleVerifier struct {
	issuer   string
	audience []string
	jwks     *keyfunc.JWKS
	now      func() time.Time
}

func NewAppleVerifier(ctx context.Context, clientIDs []string, jwksURL string) (*AppleVerifier, error) {
	if len(clientIDs) == 0 {
		return nil, errors.New("auth: apple verifier requires at least one client id")
	}
	if jwksURL == "" {
		jwksURL = defaultAppleJWKS
	}

	jwks, err := keyfunc.Get(jwksURL, keyfunc.Options{
		Ctx:              ctx,
		RefreshInterval:  time.Hour,
		RefreshTimeout:   10 * time.Second,
		RefreshRateLimit: time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: apple load jwks: %w", err)
	}

	return &AppleVerifier{
		issuer:   defaultAppleIssuer,
		audience: clientIDs,
		jwks:     jwks,
		now:      time.Now,
	}, nil
}

func (v *AppleVerifier) Provider() string {
	return providerApple
}

func (v *AppleVerifier) Verify(ctx context.Context, rawToken string) (Identity, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Identity{}, fmt.Errorf("%w: token is empty", ErrInvalidToken)
	}

	var claims appleClaims
	parsed, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		v.jwks.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience...),
		jwt.WithLeeway(60*time.Second),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !parsed.Valid {
		return Identity{}, fmt.Errorf("%w: signature verification failed", ErrInvalidToken)
	}
	if claims.Subject == "" {
		return Identity{}, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	identity := Identity{
		Provider:       providerApple,
		ProviderUserID: claims.Subject,
		Email:          email,
		EmailVerified:  email != "",
		Nonce:          claims.Nonce,
	}

	if identity.Nonce != "" {
		if err := verifyNonce(rawNonceFromContext(ctx), identity.Nonce); err != nil {
			return Identity{}, err
		}
	}

	return identity, nil
}

func verifyNonce(rawNonce, tokenNonce string) error {
	if strings.TrimSpace(rawNonce) == "" {
		return fmt.Errorf("%w: nonce claim present but no nonce supplied", ErrInvalidToken)
	}
	sum := sha256.Sum256([]byte(rawNonce))
	expected := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(expected), []byte(tokenNonce)) != 1 {
		return fmt.Errorf("%w: nonce mismatch", ErrInvalidToken)
	}
	return nil
}

func WithRawNonce(ctx context.Context, rawNonce string) context.Context {
	return context.WithValue(ctx, rawNonceKey{}, rawNonce)
}

func rawNonceFromContext(ctx context.Context) string {
	nonce, _ := ctx.Value(rawNonceKey{}).(string)
	return nonce
}

type rawNonceKey struct{}

type appleClaims struct {
	jwt.RegisteredClaims
	Email          string `json:"email"`
	IsPrivateEmail bool   `json:"is_private_email"`
	Nonce          string `json:"nonce"`
}
