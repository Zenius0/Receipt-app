package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testClientID  = "1234567890-test.apps.googleusercontent.com"
	testOtherID   = "9876543210-other.apps.googleusercontent.com"
	testSubject   = "123456789012345678901"
	testKeyID     = "test-key-1"
	testIssuer    = "https://accounts.google.com"
	testEmail     = "Selim@example.com"
	testSignedSub = "signed"
)

// testKeyPair, RSA anahtar çifti ve ona ait JWKS sunucusu döndürür.
func testKeyPair(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	modulus := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	exponent := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
	jwks := fmt.Sprintf(
		`{"keys":[{"kty":"RSA","alg":"RS256","use":"sig","kid":%q,"n":%q,"e":%q}]}`,
		testKeyID, modulus, exponent,
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwks))
	}))
	t.Cleanup(server.Close)

	return key, server.URL
}

func testVerifier(t *testing.T, jwksURL string, clientIDs ...string) *GoogleVerifier {
	t.Helper()

	verifier, err := NewGoogleVerifier(context.Background(), clientIDs, jwksURL)
	if err != nil {
		t.Fatalf("NewGoogleVerifier: %v", err)
	}
	return verifier
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return signed
}

func baseClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":            testIssuer,
		"sub":            testSubject,
		"aud":            testClientID,
		"azp":            testClientID,
		"email":          testEmail,
		"email_verified": true,
		"name":           "Selim Aydın",
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"exp":            time.Now().Add(time.Hour).Unix(),
	}
}

func TestGoogleVerifier_AcceptsValidToken(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	identity, err := verifier.Verify(context.Background(), signTestToken(t, key, baseClaims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if identity.Provider != providerGoogle {
		t.Errorf("provider = %q, want %q", identity.Provider, providerGoogle)
	}
	if identity.ProviderUserID != testSubject {
		t.Errorf("subject = %q, want %q", identity.ProviderUserID, testSubject)
	}
	if identity.Email != "selim@example.com" {
		t.Errorf("email = %q, want %q", identity.Email, "selim@example.com")
	}
	if !identity.EmailVerified {
		t.Error("email_verified = false, want true")
	}
	if identity.Name != "Selim Aydın" {
		t.Errorf("name = %q, want %q", identity.Name, "Selim Aydın")
	}
}

func TestGoogleVerifier_RejectsDisallowedAudience(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	claims["aud"] = testOtherID
	claims["azp"] = testOtherID

	_, err := verifier.Verify(context.Background(), signTestToken(t, key, claims))
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// Mobil istemci webClientId kullandığı için aud web istemcisine, azp ise
// Android istemcisine işaret edebilir. azp izinliyse token kabul edilmeli.
func TestGoogleVerifier_AcceptsAllowedAuthorizedParty(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	claims["aud"] = testOtherID
	claims["azp"] = testClientID

	if _, err := verifier.Verify(context.Background(), signTestToken(t, key, claims)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestGoogleVerifier_RejectsExpiredToken(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	claims["exp"] = time.Now().Add(-2 * time.Hour).Unix()

	if _, err := verifier.Verify(context.Background(), signTestToken(t, key, claims)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestGoogleVerifier_RejectsForeignSignature(t *testing.T) {
	_, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), signTestToken(t, otherKey, baseClaims())); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestGoogleVerifier_RejectsUnexpectedIssuer(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	claims["iss"] = "https://accounts.evil.example"

	if _, err := verifier.Verify(context.Background(), signTestToken(t, key, claims)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestGoogleVerifier_RejectsUnverifiedEmail(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	claims["email_verified"] = false

	if _, err := verifier.Verify(context.Background(), signTestToken(t, key, claims)); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("err = %v, want ErrEmailNotVerified", err)
	}
}

func TestGoogleVerifier_RejectsMissingEmail(t *testing.T) {
	key, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	claims := baseClaims()
	delete(claims, "email")

	if _, err := verifier.Verify(context.Background(), signTestToken(t, key, claims)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestGoogleVerifier_RejectsEmptyToken(t *testing.T) {
	_, jwksURL := testKeyPair(t)
	verifier := testVerifier(t, jwksURL, testClientID)

	if _, err := verifier.Verify(context.Background(), "   "); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestGoogleVerifier_RequiresClientIDs(t *testing.T) {
	if _, err := NewGoogleVerifier(context.Background(), nil, "https://example.test/certs"); err == nil {
		t.Fatal("expected error for empty client id list")
	}
}
