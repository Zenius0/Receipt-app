package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/selim/fatura-backend/internal/auth"
)

type contextKey string

const sessionClaimsKey contextKey = "session-claims"

type Auth struct {
	issuer *auth.TokenIssuer
}

func NewAuth(issuer *auth.TokenIssuer) *Auth {
	return &Auth{issuer: issuer}
}

func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken, ok := bearerToken(r)
		if !ok {
			unauthorized(w)
			return
		}

		claims, err := a.issuer.Parse(rawToken)
		if err != nil {
			unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), sessionClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ClaimsFromContext(ctx context.Context) (*auth.SessionClaims, bool) {
	claims, ok := ctx.Value(sessionClaimsKey).(*auth.SessionClaims)
	return claims, ok
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="fatura-api"`)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"missing or invalid session token"}`))
}
