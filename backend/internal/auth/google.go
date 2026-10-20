package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/golang-jwt/jwt/v5"
)

const (
	providerGoogle    = "google"
	defaultGoogleJWKS = "https://www.googleapis.com/oauth2/v3/certs"
)

// Google id_token'ları iki biçimde issuer kullanır.
var googleIssuers = []string{"accounts.google.com", "https://accounts.google.com"}

// GoogleVerifier, Google'ın imzaladığı id_token'ı JWKS üzerinden doğrular.
//
// Neden tokeninfo değil: oauth2.googleapis.com/tokeninfo legacy bir uç
// noktadır ve native (Android/iOS) istemciler için üretilen id_token'ları
// "invalid_token" ile reddedebilir. Mobil uygulamadan gelen token da tam olarak
// bu türden olduğu için doğrulama başarısız oluyordu. Apple tarafında zaten
// kullanılan JWKS + RS256 yöntemi hem bu sorunu ortadan kaldırır hem de
// hata mesajlarını çok daha açık hale getirir.
type GoogleVerifier struct {
	clientIDs map[string]struct{}
	jwks      *keyfunc.JWKS
	now       func() time.Time
}

func NewGoogleVerifier(ctx context.Context, clientIDs []string, jwksURL string) (*GoogleVerifier, error) {
	if len(clientIDs) == 0 {
		return nil, errors.New("auth: google verifier requires at least one client id")
	}
	if jwksURL == "" {
		jwksURL = defaultGoogleJWKS
	}

	allowed := make(map[string]struct{}, len(clientIDs))
	for _, id := range clientIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	jwks, err := keyfunc.Get(jwksURL, keyfunc.Options{
		Ctx:              ctx,
		RefreshInterval:  time.Hour,
		RefreshTimeout:   10 * time.Second,
		RefreshRateLimit: time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: google load jwks: %w", err)
	}

	return &GoogleVerifier{
		clientIDs: allowed,
		jwks:      jwks,
		now:       time.Now,
	}, nil
}

func (v *GoogleVerifier) Provider() string {
	return providerGoogle
}

func (v *GoogleVerifier) Verify(_ context.Context, rawToken string) (Identity, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Identity{}, fmt.Errorf("%w: token is empty", ErrInvalidToken)
	}

	var claims googleClaims
	parsed, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		v.jwks.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
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

	// jwt/v5 tek issuer kabul ettiği için issuer'ı elle doğruluyoruz.
	// İmza bu noktadan önce zaten doğrulanmış oluyor.
	if !isGoogleIssuer(claims.Issuer) {
		return Identity{}, fmt.Errorf("%w: unexpected issuer %q", ErrInvalidToken, claims.Issuer)
	}

	// Google'da aud (token'ın verildiği istemci) ile azp (yetkili taraf)
	// farklı olabilir. Mobil istemci webClientId kullandığı için aud web
	// istemcisidir; bazı akışlarda ise aud Android istemcisine, azp web
	// istemcisine işaret eder. İkisinden biri izinliyse token kabul edilir.
	audience := []string(claims.Audience)
	if len(audience) == 0 && claims.AuthorizedParty == "" {
		return Identity{}, fmt.Errorf("%w: missing audience", ErrInvalidToken)
	}
	if !v.audienceAllowed(audience, claims.AuthorizedParty) {
		return Identity{}, fmt.Errorf(
			"%w: audience %q is not allowed (azp %q)",
			ErrInvalidToken,
			strings.Join(audience, ","),
			claims.AuthorizedParty,
		)
	}

	if claims.Subject == "" {
		return Identity{}, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}
	if claims.ExpiresAt == nil {
		return Identity{}, fmt.Errorf("%w: missing expiry", ErrInvalidToken)
	}
	if claims.ExpiresAt.Time.Before(v.now()) {
		return Identity{}, fmt.Errorf("%w: token expired", ErrInvalidToken)
	}

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" {
		return Identity{}, fmt.Errorf("%w: missing email claim", ErrInvalidToken)
	}
	if !claims.isEmailVerified() {
		return Identity{}, ErrEmailNotVerified
	}

	return Identity{
		Provider:       providerGoogle,
		ProviderUserID: claims.Subject,
		Email:          email,
		EmailVerified:  true,
		Name:           firstNonEmpty(claims.Name, strings.TrimSpace(claims.GivenName+" "+claims.FamilyName)),
	}, nil
}

// isGoogleIssuer, token'ın Google tarafından imzalandığını doğrular.
func isGoogleIssuer(issuer string) bool {
	for _, allowed := range googleIssuers {
		if issuer == allowed {
			return true
		}
	}
	return false
}

// audienceAllowed, aud ya da azp izinli istemci listesinde mi diye bakar.
func (v *GoogleVerifier) audienceAllowed(audience []string, authorizedParty string) bool {
	for _, aud := range audience {
		if _, ok := v.clientIDs[strings.TrimSpace(aud)]; ok {
			return true
		}
	}
	if authorizedParty != "" {
		if _, ok := v.clientIDs[strings.TrimSpace(authorizedParty)]; ok {
			return true
		}
	}
	return false
}

type googleClaims struct {
	jwt.RegisteredClaims
	Email           string          `json:"email"`
	EmailVerified   json.RawMessage `json:"email_verified"`
	Name            string          `json:"name"`
	GivenName       string          `json:"given_name"`
	FamilyName      string          `json:"family_name"`
	AuthorizedParty string          `json:"azp"`
}

func (g googleClaims) isEmailVerified() bool {
	raw := strings.Trim(strings.TrimSpace(string(g.EmailVerified)), `"`)
	if raw == "" {
		return false
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
