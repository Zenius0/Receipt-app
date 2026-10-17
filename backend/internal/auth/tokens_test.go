package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/store"
)

func testUser() store.User {
	name := "Selim Aydin"
	return store.User{
		ID:              uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:           "selim@example.com",
		Name:            &name,
		Provider:        "google",
		AccountantEmail: nil,
		CreatedAt:       time.Now(),
	}
}

func newTestIssuer(t *testing.T, ttl time.Duration) *TokenIssuer {
	t.Helper()
	issuer, err := NewTokenIssuer([]byte(strings.Repeat("s", 48)), ttl)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	return issuer
}

func TestTokenIssuer_RoundTrip(t *testing.T) {
	issuer := newTestIssuer(t, time.Hour)
	user := testUser()

	token, expiresAt, err := issuer.Issue(user)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expected future expiry, got %v", expiresAt)
	}

	claims, err := issuer.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != user.ID {
		t.Fatalf("user id mismatch: got %v want %v", claims.UserID, user.ID)
	}
	if claims.Email != user.Email {
		t.Fatalf("email mismatch: got %q want %q", claims.Email, user.Email)
	}
	if claims.Provider != user.Provider {
		t.Fatalf("provider mismatch: got %q want %q", claims.Provider, user.Provider)
	}
	if claims.ID == "" {
		t.Fatal("expected jti claim")
	}
}

func TestTokenIssuer_RejectsForeignSignature(t *testing.T) {
	issuer := newTestIssuer(t, time.Hour)
	other, _ := NewTokenIssuer([]byte(strings.Repeat("x", 48)), time.Hour)

	token, _, err := other.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := issuer.Parse(token); err == nil {
		t.Fatal("expected token signed with another secret to be rejected")
	}
}

func TestTokenIssuer_RejectsExpired(t *testing.T) {
	issuer := newTestIssuer(t, time.Hour)
	issuer.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	issuer.ttl = time.Minute

	token, _, err := issuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	issuer.now = time.Now
	if _, err := issuer.Parse(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestTokenIssuer_RejectsUnsignedToken(t *testing.T) {
	issuer := newTestIssuer(t, time.Hour)

	if _, err := issuer.Parse("not-a-jwt"); err == nil {
		t.Fatal("expected malformed token to be rejected")
	}
}

func TestNewTokenIssuer_ValidatesInput(t *testing.T) {
	if _, err := NewTokenIssuer([]byte("short"), time.Hour); err == nil {
		t.Fatal("expected short signing secret to be rejected")
	}
	if _, err := NewTokenIssuer([]byte(strings.Repeat("s", 48)), 0); err == nil {
		t.Fatal("expected zero ttl to be rejected")
	}
}
