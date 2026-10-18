package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env               string
	Port              string
	DatabaseURL       string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	RequestTimeout    time.Duration
	AllowedOrigins    []string
	AccessTokenTTL    time.Duration
	JWTSecret         []byte
	GoogleClientIDs   []string
	AppleClientIDs    []string

	// AI (fiş görseli okuma) yapılandırması.
	AIProvider       string
	GeminiAPIKey     string
	GeminiModel      string
	OpenAIAPIKey     string
	OpenAIModel      string
	ScanTimeout      time.Duration
	MaxScanImageSize int
}

func Load() (Config, error) {
	cfg := Config{
		Env:               getString("APP_ENV", "development"),
		Port:              getString("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		ShutdownTimeout:   getDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		ReadHeaderTimeout: getDuration("READ_HEADER_TIMEOUT", 5*time.Second),
		RequestTimeout:    getDuration("REQUEST_TIMEOUT", 15*time.Second),
		AllowedOrigins:    getList("ALLOWED_ORIGINS"),
		AccessTokenTTL:    getDuration("ACCESS_TOKEN_TTL", 24*time.Hour),
		JWTSecret:         []byte(os.Getenv("JWT_SECRET")),
		GoogleClientIDs:   getList("GOOGLE_CLIENT_IDS"),
		AppleClientIDs:    getList("APPLE_CLIENT_IDS"),

		AIProvider:       strings.ToLower(getString("AI_PROVIDER", "gemini")),
		GeminiAPIKey:     strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:      getString("GEMINI_MODEL", "gemini-3.8-flash"),
		OpenAIAPIKey:     strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:      getString("OPENAI_MODEL", "gpt-4o"),
		ScanTimeout:      getDuration("SCAN_TIMEOUT", 45*time.Second),
		MaxScanImageSize: getInt("MAX_SCAN_IMAGE_BYTES", 8<<20),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("config: DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("config: JWT_SECRET must be at least 32 bytes")
	}
	// Apple Developer üyeliği yıllık ücretli olduğu için Apple kimliği zorunlu
	// tutulmaz: en az bir giriş sağlayıcısı yapılandırılmış olmalıdır.
	// Yapılandırılmayan sağlayıcının ucu 404 unsupported_provider döner ve
	// mobil istemci o giriş butonunu göstermez.
	if len(cfg.GoogleClientIDs) == 0 && len(cfg.AppleClientIDs) == 0 {
		return Config{}, errors.New("config: at least one of GOOGLE_CLIENT_IDS or APPLE_CLIENT_IDS is required")
	}
	if cfg.AccessTokenTTL > 30*24*time.Hour {
		return Config{}, errors.New("config: ACCESS_TOKEN_TTL must not exceed 720h")
	}
	if err := cfg.validateAI(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validateAI, seçili sağlayıcının yapılandırmasının eksiksiz olduğunu doğrular.
// Eksik anahtar sunucu başlangıcında hata vermek üzeredir: bu, üretimdeki ilk
// fiş taramasında 500 dönmekten çok iyidir.
func (c Config) validateAI() error {
	switch c.AIProvider {
	case "gemini":
		if c.GeminiAPIKey == "" {
			return errors.New("config: GEMINI_API_KEY is required when AI_PROVIDER=gemini")
		}
	case "openai":
		if c.OpenAIAPIKey == "" {
			return errors.New("config: OPENAI_API_KEY is required when AI_PROVIDER=openai")
		}
	default:
		return fmt.Errorf("config: AI_PROVIDER must be 'gemini' or 'openai', got %q", c.AIProvider)
	}

	if c.ScanTimeout <= 0 {
		return errors.New("config: SCAN_TIMEOUT must be positive")
	}
	if c.MaxScanImageSize <= 0 {
		return errors.New("config: MAX_SCAN_IMAGE_BYTES must be positive")
	}

	return nil
}

// EffectiveWriteTimeout, HTTP sunucusunun yazma zaman aşımını hesaplar.
// Fiş tarama uçları bir yapay zekâ çağrısı yaptığı için REQUEST_TIMEOUT'tan
// uzun sürebilir; aksi halde sunucu yanıtı yazmadan bağlantıyı keser.
func (c Config) EffectiveWriteTimeout() time.Duration {
	timeout := c.RequestTimeout
	if minimum := c.ScanTimeout + 5*time.Second; minimum > timeout {
		timeout = minimum
	}
	return timeout
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

func getString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func getDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func getInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
