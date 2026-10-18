package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/selim/fatura-backend/internal/ratelimit"
)

// RateLimit, IP başına saniyede N isteğe izin veren genel korumadır.
// Kimlik doğrulama yapılmadan önce çalışır: botun oturum edinmesine
// gerek kalmadan durdurulmasını sağlar.
type RateLimit struct {
	limiter *ratelimit.Limiter
	logger  *slog.Logger
}

func NewRateLimit(limiter *ratelimit.Limiter, logger *slog.Logger) *RateLimit {
	if logger == nil {
		logger = slog.Default()
	}
	return &RateLimit{limiter: limiter, logger: logger}
}

func (rl *RateLimit) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := rl.limiter.Allow(clientIP(r))
		setRateHeaders(w, result)

		if !result.Allowed {
			rl.logger.WarnContext(r.Context(), "rate limit exceeded",
				slog.String("ip", clientIP(r)),
				slog.String("path", r.URL.Path),
				slog.Int64("retry_after_seconds", int64(result.RetryAfter.Seconds())),
			)
			writeTooManyRequests(w, result, "rate_limited", "çok fazla istek gönderildi, biraz bekleyin")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ScanQuota, kullanıcı başına günde N fiş taramasına izin verir.
// Kimlik doğrulamasın DAN SONRA çalışır ve kotaları kullanıcı bazlıdır;
// IP kotası paylaşılan Wi-Fi arkasında kullanıcıları haksız yere keserdi.
//
// Bu, yapay zekâ çağrısının maliyetini koruyan asıl sınırdır: limit
// uygulanmazsa istemci keyfi sayıda fiş tarayıp ücret üretebilirdi.
type ScanQuota struct {
	limiter *ratelimit.Limiter
	logger  *slog.Logger
}

func NewScanQuota(limiter *ratelimit.Limiter, logger *slog.Logger) *ScanQuota {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScanQuota{limiter: limiter, logger: logger}
}

func (q *ScanQuota) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			// JWT guard bu noktadan önce çalışmamış; güvenli tarafta kal.
			unauthorized(w)
			return
		}

		key := "scan:" + claims.UserID.String()
		result := q.limiter.Allow(key)
		setRateHeaders(w, result)

		if !result.Allowed {
			q.logger.WarnContext(r.Context(), "daily scan quota exceeded",
				slog.String("user_id", claims.UserID.String()),
				slog.Int("daily_limit", result.Limit),
				slog.Int64("retry_after_seconds", int64(result.RetryAfter.Seconds())),
				slog.String("resets_at", result.ResetAt.UTC().Format(time.RFC3339)),
			)
			writeTooManyRequests(w, result, "quota_exceeded",
				fmt.Sprintf("günlük %d tarama hakkınız doldu, yarın tekrar deneyin", result.Limit))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setRateHeaders, istemciye (ve testlere) kalan hakkı ve sıfırlanma zamanını
// bildirir.
func setRateHeaders(w http.ResponseWriter, result ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", result.ResetAt.UTC().Format(time.RFC3339))
	if !result.Allowed && result.RetryAfter > 0 {
		seconds := int(result.RetryAfter.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
}

func writeTooManyRequests(w http.ResponseWriter, result ratelimit.Result, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)

	// Retry-After başlığı yukarıda ayarlandı; gövde yalnızca makine-okur.
	_, _ = fmt.Fprintf(w, `{"error":%q,"message":%q}`, code, message)
}

// clientIP, ters vekil arkasındaki gerçek istemci adresini bulmaya çalışır.
// Render bir proxy arkasında olduğu için X-Forwarded-For güvenilir kabul edilir;
// bunu yalnızca herkesin erişebildiği bir servis uçlarında kullandığınızdan emin olun.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// "client, proxy1, proxy2" biçiminde ilk eleman istemcidir.
		if idx := strings.IndexByte(forwarded, ','); idx >= 0 {
			forwarded = forwarded[:idx]
		}
		if ip := strings.TrimSpace(forwarded); ip != "" {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
