package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/config"
	"github.com/selim/fatura-backend/internal/httpapi"
	"github.com/selim/fatura-backend/internal/platform/db"
	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store/pgstore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	// Yalnızca yapılandırılmış sağlayıcılar kayıt altına alınır. Apple
	// Developer üyeliği ücretli olduğu için Apple kimliği olmadan da Google
	// ile giriş yapılabilir; Apple ucu yapılandırılmamışsa 404 döner.
	verifiers, err := buildVerifiers(ctx, cfg)
	if err != nil {
		return err
	}
	for _, verifier := range verifiers {
		logger.Info("auth provider enabled", slog.String("provider", verifier.Provider()))
	}

	tokenIssuer, err := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	userRepository := pgstore.NewUserRepository(pool)

	authService, err := auth.NewService(userRepository, tokenIssuer, verifiers...)
	if err != nil {
		return err
	}

	scanner, err := newScanner(cfg)
	if err != nil {
		return err
	}
	logger.Info("ai scanner ready", slog.String("provider", scanner.Name()))

	receiptRepository := pgstore.NewReceiptRepository(pool)

	receiptService, err := receipts.NewService(receiptRepository, scanner, logger)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.NewRouter(httpapi.Dependencies{
			Logger:         logger,
			AuthService:    authService,
			TokenIssuer:    tokenIssuer,
			ReceiptService: receiptService,
			AllowedOrigins: cfg.AllowedOrigins,
			MaxScanImage:   cfg.MaxScanImageSize,
			ReadinessCheck: func() error {
				pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				return pool.Ping(pingCtx)
			},
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.RequestTimeout,
		// Fiş tarama bir AI çağrısı yaptığından yazma zaman aşımı REQUEST_TIMEOUT
		//'tan daha geniş olmalı; aksi halde sunucu yanıt yazmadan bağlantıyı keser.
		WriteTimeout: cfg.EffectiveWriteTimeout(),
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening",
			slog.String("addr", server.Addr),
			slog.String("env", cfg.Env),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("server stopped")
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	if !cfg.IsProduction() {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}

	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// newScanner, AI_PROVIDER ayarına göre fiş görseli okuyan servisi kurar.
// Sağlayıcı seçimi burada toplanır; handler katmanı Gemini/OpenAI'yi bilmez.
func newScanner(cfg config.Config) (ai.Scanner, error) {
	switch cfg.AIProvider {
	case "openai":
		return ai.NewOpenAIScanner(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.ScanTimeout)
	default:
		return ai.NewGeminiScanner(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.ScanTimeout)
	}
}

// buildVerifiers, ortam değişkenlerinde tanımlı giriş sağlayıcılarını kurar.
// Kimlik verisi verilmemiş sağlayıcı sessizce atlanır; auth.Service o sağlayıcıya
// gelen istekleri unsupported_provider ile reddeder.
func buildVerifiers(ctx context.Context, cfg config.Config) ([]auth.Verifier, error) {
	verifiers := make([]auth.Verifier, 0, 2)

	if len(cfg.GoogleClientIDs) > 0 {
		verifier, err := auth.NewGoogleVerifier(cfg.GoogleClientIDs, "")
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, verifier)
	}

	if len(cfg.AppleClientIDs) > 0 {
		verifier, err := auth.NewAppleVerifier(ctx, cfg.AppleClientIDs, "")
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, verifier)
	}

	return verifiers, nil
}
