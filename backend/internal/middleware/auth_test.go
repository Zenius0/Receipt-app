package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/store"
)

func testUser() store.User {
	return store.User{
		ID:    uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email: "selim@example.com",
	}
}

func newTestGuard(t *testing.T) (*Auth, string) {
	t.Helper()
	issuer, err := auth.NewTokenIssuer([]byte(strings.Repeat("s", 48)), time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	token, _, err := issuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	return NewAuth(issuer), token
}

func TestAuth_Require_PassesValidSession(t *testing.T) {
	guard, token := newTestGuard(t)

	var userID uuid.UUID
	next := guard.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("expected claims in context")
		}
		userID = claims.UserID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if userID != testUser().ID {
		t.Fatalf("unexpected user id in context: %v", userID)
	}
}

func TestAuth_Require_RejectsRequests(t *testing.T) {
	guard, token := newTestGuard(t)

	cases := map[string]string{
		"missing header":   "",
		"malformed scheme": "Basic " + token,
		"empty credential": "Bearer ",
		"invalid token":    "Bearer abc.def.ghi",
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			handler := guard.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler must not run")
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("expected WWW-Authenticate header")
			}
		})
	}
}
