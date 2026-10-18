package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/httpapi/handlers"
	"github.com/selim/fatura-backend/internal/httpx"
	"github.com/selim/fatura-backend/internal/middleware"
	"github.com/selim/fatura-backend/internal/receipts"
)

type Dependencies struct {
	Logger         *slog.Logger
	AuthService    *auth.Service
	TokenIssuer    *auth.TokenIssuer
	ReceiptService *receipts.Service
	AllowedOrigins []string
	MaxScanImage   int
	// RateLimit ve ScanQuota nil ise o koruma kapalıdır (test/geliştirme).
	RateLimit      *middleware.RateLimit
	ScanQuota      *middleware.ScanQuota
	ReadinessCheck func() error
}

func NewRouter(deps Dependencies) http.Handler {
	logger := deps.Logger

	authHandler := handlers.NewAuthHandler(deps.AuthService, logger)
	userHandler := handlers.NewUserHandler(deps.AuthService)
	authMiddleware := middleware.NewAuth(deps.TokenIssuer)
	cors := middleware.NewCORS(deps.AllowedOrigins)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if deps.ReadinessCheck != nil {
			if err := deps.ReadinessCheck(); err != nil {
				httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "database is unreachable")
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("POST /api/v1/auth/google", authHandler.Google)
	mux.HandleFunc("POST /api/v1/auth/apple", authHandler.Apple)

	mux.Handle("GET /api/v1/me", authMiddleware.Require(http.HandlerFunc(userHandler.Me)))

	// Fiş uçları hem JWT hem de yapılandırılmış bir AI sağlayıcısı gerektirir.
	if deps.ReceiptService != nil {
		receiptHandler := handlers.NewReceiptHandler(deps.ReceiptService, logger, deps.MaxScanImage)

		// Sıralama önemlidir:
		//   1) JWT guard DIŞTA — oturum doğrulanır ve kullanıcı kimliği
		//      request context'ine yazılır.
		//   2) Günlük tarama kotası İÇERİDE — kimliği context'ten okur ve
		//      AI çağrısından hemen önce çalışır, böylece kota aşıldığında
		//      ücret harcanmaz.
		// Tersi (kota dışarıda) 401'e yol açar: context henüz doldurulmamıştır.
		scan := http.Handler(http.HandlerFunc(receiptHandler.Scan))
		if deps.ScanQuota != nil {
			scan = deps.ScanQuota.Handler(scan)
		}

		mux.Handle("POST /api/v1/receipts/scan", authMiddleware.Require(scan))
		mux.Handle("POST /api/v1/receipts/confirm", authMiddleware.Require(http.HandlerFunc(receiptHandler.Confirm)))
		mux.Handle("GET /api/v1/receipts", authMiddleware.Require(http.HandlerFunc(receiptHandler.List)))
	} else {
		logger.Warn("receipt routes disabled: receipt service is not configured")
	}

	// Middleware zinciri: Chain(handler, mw...) içte dışa sarar, yani
	// listedeki ilk middleware en dışta çalışır.
	middlewares := []func(http.Handler) http.Handler{
		middleware.RequestID(),
		middleware.RequestLogger(logger),
		middleware.Recoverer(logger),
	}
	// Genel IP limiti en dış katmanda: oturum edinmeye çalışan botlar
	// veritabanına veya yapay zekâya ulaşmadan elenir.
	if deps.RateLimit != nil {
		middlewares = append(middlewares, deps.RateLimit.Handler)
	}
	middlewares = append(middlewares,
		middleware.SecurityHeaders,
		cors.Handler,
	)

	return middleware.Chain(mux, middlewares...)
}
