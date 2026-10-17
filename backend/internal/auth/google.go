package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	providerGoogle       = "google"
	defaultTokenInfoHost = "https://oauth2.googleapis.com/tokeninfo"
)

var googleIssuers = map[string]struct{}{
	"accounts.google.com":         {},
	"https://accounts.google.com": {},
}

type GoogleVerifier struct {
	clientIDs map[string]struct{}
	endpoint  string
	http      *http.Client
}

func NewGoogleVerifier(clientIDs []string, endpoint string) (*GoogleVerifier, error) {
	if len(clientIDs) == 0 {
		return nil, errors.New("auth: google verifier requires at least one client id")
	}
	if endpoint == "" {
		endpoint = defaultTokenInfoHost
	}

	allowed := make(map[string]struct{}, len(clientIDs))
	for _, id := range clientIDs {
		allowed[strings.TrimSpace(id)] = struct{}{}
	}

	return &GoogleVerifier{
		clientIDs: allowed,
		endpoint:  endpoint,
		http:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (v *GoogleVerifier) Provider() string {
	return providerGoogle
}

func (v *GoogleVerifier) Verify(ctx context.Context, rawToken string) (Identity, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Identity{}, fmt.Errorf("%w: token is empty", ErrInvalidToken)
	}

	endpoint := v.endpoint + "?id_token=" + url.QueryEscape(rawToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: google build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := v.http.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: google request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Identity{}, fmt.Errorf("auth: google read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("%w: tokeninfo returned %d", ErrInvalidToken, res.StatusCode)
	}

	var payload googleTokenInfo
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("%w: malformed tokeninfo payload", ErrInvalidToken)
	}

	if err := v.validate(ctx, payload); err != nil {
		return Identity{}, err
	}

	return Identity{
		Provider:       providerGoogle,
		ProviderUserID: payload.Sub,
		Email:          strings.ToLower(strings.TrimSpace(payload.Email)),
		EmailVerified:  payload.isEmailVerified(),
		Name:           firstNonEmpty(payload.Name, strings.TrimSpace(payload.GivenName+" "+payload.FamilyName)),
	}, nil
}

func (v *GoogleVerifier) validate(_ context.Context, payload googleTokenInfo) error {
	if _, ok := googleIssuers[payload.Iss]; !ok {
		return fmt.Errorf("%w: unexpected issuer %q", ErrInvalidToken, payload.Iss)
	}
	if payload.Sub == "" {
		return fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}
	if payload.Aud == "" && payload.Azp == "" {
		return fmt.Errorf("%w: missing audience", ErrInvalidToken)
	}
	if _, ok := v.clientIDs[payload.Aud]; !ok {
		if _, fallback := v.clientIDs[payload.Azp]; !fallback {
			return fmt.Errorf("%w: audience %q is not allowed", ErrInvalidToken, payload.Aud)
		}
	}
	if payload.Exp == 0 {
		return fmt.Errorf("%w: missing expiry", ErrInvalidToken)
	}
	if time.Now().After(time.Unix(payload.Exp, 0)) {
		return fmt.Errorf("%w: token expired", ErrInvalidToken)
	}
	if payload.Email == "" {
		return fmt.Errorf("%w: missing email claim", ErrInvalidToken)
	}
	if !payload.isEmailVerified() {
		return ErrEmailNotVerified
	}

	return nil
}

type googleTokenInfo struct {
	Aud           string          `json:"aud"`
	Azp           string          `json:"azp"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Exp           int64           `json:"exp"`
	Iss           string          `json:"iss"`
	Sub           string          `json:"sub"`
	Name          string          `json:"name"`
	GivenName     string          `json:"given_name"`
	FamilyName    string          `json:"family_name"`
}

func (g googleTokenInfo) isEmailVerified() bool {
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
