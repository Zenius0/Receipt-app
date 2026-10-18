package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/selim/fatura-backend/internal/config"
)

// baseEnv, geçerli bir minimum ortam değişkenleri seti verir.
// t.Setenv ile ayarlananlar test bittikten sonra otomatik temizlenir.
func baseEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/fatura?sslmode=disable")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 48))
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")
	t.Setenv("GOOGLE_CLIENT_IDS", "000000000000-abc.apps.googleusercontent.com")
	t.Setenv("APPLE_CLIENT_IDS", "")
}

// Apple Developer üyeliği ücretlidir; Apple kimliği olmadan da Google ile
// giriş yapılabilmelidir.
func TestLoad_GoogleOnly(t *testing.T) {
	baseEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.GoogleClientIDs) != 1 {
		t.Errorf("GoogleClientIDs = %v, want 1 entry", cfg.GoogleClientIDs)
	}
	if len(cfg.AppleClientIDs) != 0 {
		t.Errorf("AppleClientIDs = %v, want empty", cfg.AppleClientIDs)
	}
}

func TestLoad_AppleOnly(t *testing.T) {
	baseEnv(t)
	os.Setenv("GOOGLE_CLIENT_IDS", "")
	t.Setenv("APPLE_CLIENT_IDS", "com.selim.fatura")

	if _, err := config.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// Hiçbir sağlayıcı yoksa sunucu başlamamalı: kullanıcı giriş yapamaz.
func TestLoad_NoProviderFails(t *testing.T) {
	baseEnv(t)
	os.Setenv("GOOGLE_CLIENT_IDS", "")
	os.Setenv("APPLE_CLIENT_IDS", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded without any auth provider, want error")
	}
	if !strings.Contains(err.Error(), "GOOGLE_CLIENT_IDS or APPLE_CLIENT_IDS") {
		t.Errorf("error = %v, want provider requirement message", err)
	}
}

func TestLoad_ShortJWTSecretFails(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_SECRET", "too-short")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error = %v, want JWT_SECRET length error", err)
	}
}

// Sağlayıcı olmasa bile AI yapılandırması zorunludur: fiş okuma uçları
// aksi halde sessizce devre dışı kalırdı.
func TestLoad_RequiresAIKey(t *testing.T) {
	baseEnv(t)
	t.Setenv("GEMINI_API_KEY", "")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Errorf("error = %v, want GEMINI_API_KEY required error", err)
	}
}

func TestLoad_InvalidAIProvider(t *testing.T) {
	baseEnv(t)
	t.Setenv("AI_PROVIDER", "anthropic")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "AI_PROVIDER") {
		t.Errorf("error = %v, want AI_PROVIDER validation error", err)
	}
}

func TestEffectiveWriteTimeout_CoversScan(t *testing.T) {
	baseEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// AI çağrısı REQUEST_TIMEOUT'tan uzun sürebildiği için yazma zaman aşımı
	// bunu aşmak zorunda.
	if got := cfg.EffectiveWriteTimeout(); got <= cfg.ScanTimeout {
		t.Errorf("EffectiveWriteTimeout = %v, must exceed ScanTimeout %v", got, cfg.ScanTimeout)
	}
}
