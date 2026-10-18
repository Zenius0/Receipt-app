package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/httpapi"
	"github.com/selim/fatura-backend/internal/middleware"
	"github.com/selim/fatura-backend/internal/ratelimit"
)

// newLimitedHarness, tarama kotası takılı bir router kurar.
func newLimitedHarness(t *testing.T, dailyLimit int) *harness {
	t.Helper()

	quota := ratelimit.New(dailyLimit, 24*time.Hour)
	if quota == nil {
		t.Fatalf("ratelimit.New(%d) nil döndü", dailyLimit)
	}

	return newHarness(t, func(d *httpapi.Dependencies) {
		d.ScanQuota = middleware.NewScanQuota(quota, discardLogger())
		d.RateLimit = middleware.NewRateLimit(ratelimit.New(10000, time.Minute), discardLogger())
	})
}

// Tarama kotası dolduğunda AI'ya GİTİLMEMELİ — ücret harcanmasın.
func TestScan_DailyQuotaStopsBeforeAI(t *testing.T) {
	h := newLimitedHarness(t, 2)
	h.scanner.result = ai.ScanResult{
		StoreName:   "Shell",
		TotalAmount: 100,
		Currency:    "TRY",
		Category:    "fuel",
		ReceiptDate: nowUTC(),
	}

	for i := 0; i < 2; i++ {
		body, contentType := multipartBody(t, "image", "r.png", tinyPNG)
		rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("tarama %d başarısız: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}

	body, contentType := multipartBody(t, "image", "r.png", tinyPNG)
	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, true)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Error != "quota_exceeded" {
		t.Errorf("error = %q, want quota_exceeded", response.Error)
	}
	// Kullanıcı hangi sınırı aştığını anlamalı.
	if response.Message == "" {
		t.Error("message boş; kullanıcı sınırı bilmemeli")
	}

	// Kritik: kota aşıldığında AI çağrısı yapılmamalı.
	if calls := h.scanner.calls; calls != 2 {
		t.Errorf("AI çağrısı = %d, want 2 (kota aşınca artmamalı)", calls)
	}

	// Retry-After başlığı sunucu istemciye süreyi bildirmeli.
	if header(rec, "Retry-After") == "" {
		t.Error("Retry-After başlığı eksik")
	}
	if got := header(rec, "X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want 0", got)
	}
}

// Kota yalnızca scan ucunda olmalı; confirm ve list etkilenmemeli.
func TestScan_QuotaDoesNotBlockOtherEndpoints(t *testing.T) {
	h := newLimitedHarness(t, 1)
	h.scanner.result = ai.ScanResult{
		StoreName:   "Shell",
		TotalAmount: 100,
		Currency:    "TRY",
		Category:    "fuel",
		ReceiptDate: nowUTC(),
	}

	body, contentType := multipartBody(t, "image", "r.png", tinyPNG)
	scanRec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, true)
	if scanRec.Code != http.StatusOK {
		t.Fatalf("kota dolmadan tarama yapılmalıydı: %d %s", scanRec.Code, scanRec.Body.String())
	}

	// Kota doldu ama confirm ve list hâlâ çalışmalı — kota yalnızca
	// ücretli AI çağrısını korur, veri kaybına yol açmaz.
	confirmBody := `{"storeName":"Shell","totalAmount":100,"currency":"TRY","category":"fuel","receiptDate":"` +
		nowUTC().Format("2006-01-02T15:04:05Z") + `"}`
	rec := h.do(t, http.MethodPost, "/api/v1/receipts/confirm", "application/json", stringReader(confirmBody), true)
	if rec.Code != http.StatusCreated {
		t.Errorf("confirm status = %d, want 201 (kota confirm'u etkilememeli)", rec.Code)
	}

	listRec := h.do(t, http.MethodGet, "/api/v1/receipts", "", nil, true)
	if listRec.Code != http.StatusOK {
		t.Errorf("list status = %d, want 200", listRec.Code)
	}
}

// Yetkisiz istek kota sayacını tüketmemeli: IP limiti yakalarsa bile,
// kullanıcı kotası korunmalı.
func TestScan_QuotaNotConsumedWithoutAuth(t *testing.T) {
	h := newLimitedHarness(t, 3)

	body, contentType := multipartBody(t, "image", "r.png", tinyPNG)
	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if h.scanner.calls != 0 {
		t.Error("yetkisiz istek AI'ya ulaşmamalı")
	}
}
