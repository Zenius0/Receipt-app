package handlers_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/httpapi"
	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store"
)

// 1x1 saydam PNG.
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

type stubScanner struct {
	result ai.ScanResult
	err    error
	calls  int
}

func (s *stubScanner) Name() string { return "stub:model" }

func (s *stubScanner) Scan(_ context.Context, _ ai.Image) (ai.ScanResult, error) {
	s.calls++
	return s.result, s.err
}

type stubRepo struct {
	created []store.Receipt
}

func (r *stubRepo) Create(_ context.Context, receipt store.Receipt) (store.Receipt, error) {
	receipt.CreatedAt = time.Now().UTC()
	r.created = append(r.created, receipt)
	return receipt, nil
}

type harness struct {
	server  http.Handler
	token   string
	user    store.User
	scanner *stubScanner
	repo    *stubRepo
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	issuer, err := auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	user := store.User{
		ID:       uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		Email:    "selim@example.com",
		Provider: "google",
	}
	token, _, err := issuer.Issue(user)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	scanner := &stubScanner{}
	repo := &stubRepo{}

	service, err := receipts.NewService(repo, scanner, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return &harness{
		server: httpapi.NewRouter(httpapi.Dependencies{
			Logger:         logger,
			TokenIssuer:    issuer,
			ReceiptService: service,
			MaxScanImage:   ai.MaxImageBytes,
		}),
		token:   token,
		user:    user,
		scanner: scanner,
		repo:    repo,
	}
}

func (h *harness) do(t *testing.T, method, path, contentType string, body io.Reader, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}

	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	return rec
}

func multipartBody(t *testing.T, field, filename string, data []byte) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	return &buf, writer.FormDataContentType()
}

func TestScan_RequiresAuth(t *testing.T) {
	h := newHarness(t)
	body, contentType := multipartBody(t, "image", "receipt.png", tinyPNG)

	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, false)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if h.scanner.calls != 0 {
		t.Error("yetkisiz istek AI'ya ulaşmamalı")
	}
}

func TestScan_Multipart(t *testing.T) {
	h := newHarness(t)
	h.scanner.result = ai.ScanResult{
		StoreName:   "Shell Petrol",
		TotalAmount: 1500.25,
		Currency:    "TRY",
		Category:    store.CategoryFuel,
		ReceiptDate: time.Now().UTC().AddDate(0, 0, -1),
	}

	body, contentType := multipartBody(t, "image", "receipt.png", tinyPNG)
	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-AI-Model"); got != "stub:model" {
		t.Errorf("X-AI-Model = %q, want stub:model", got)
	}

	var response struct {
		StoreName   string  `json:"storeName"`
		TotalAmount float64 `json:"totalAmount"`
		Currency    string  `json:"currency"`
		Category    string  `json:"category"`
		ReceiptDate string  `json:"receiptDate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.StoreName != "Shell Petrol" || response.TotalAmount != 1500.25 || response.Category != "fuel" {
		t.Errorf("unexpected response: %+v", response)
	}
}

func TestScan_Base64JSON(t *testing.T) {
	h := newHarness(t)
	h.scanner.result = ai.ScanResult{
		StoreName:   "A101",
		TotalAmount: 250,
		Currency:    "TRY",
		Category:    store.CategoryOther,
		ReceiptDate: time.Now().UTC(),
	}

	payload := map[string]string{
		"imageBase64": "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG),
		"mimeType":    "image/png",
	}
	body, _ := json.Marshal(payload)

	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", "application/json", bytes.NewReader(body), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if h.scanner.calls != 1 {
		t.Errorf("scanner calls = %d, want 1", h.scanner.calls)
	}
}

func TestScan_RejectsNonImage(t *testing.T) {
	h := newHarness(t)

	payload := map[string]string{"imageBase64": base64.StdEncoding.EncodeToString([]byte("this is not an image"))}
	body, _ := json.Marshal(payload)

	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", "application/json", bytes.NewReader(body), true)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415, body = %s", rec.Code, rec.Body.String())
	}
	if h.scanner.calls != 0 {
		t.Error("geçersiz görsel AI'ya gönderilmemeli")
	}
}

func TestScan_NotAReceiptPropagatesReason(t *testing.T) {
	h := newHarness(t)
	h.scanner.err = &ai.NotReceiptError{Reason: "Bu görsel bir manzara fotoğrafı."}

	body, contentType := multipartBody(t, "image", "photo.png", tinyPNG)
	rec := h.do(t, http.MethodPost, "/api/v1/receipts/scan", contentType, body, true)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "not_a_receipt" {
		t.Errorf("error = %q, want not_a_receipt", response.Error)
	}
	if response.Message != "Bu görsel bir manzara fotoğrafı." {
		t.Errorf("message = %q, want model açıklaması", response.Message)
	}
}

func TestConfirm_SavesReceiptForAuthenticatedUser(t *testing.T) {
	h := newHarness(t)

	imagePath := "file:///data/user/0/com.fatura/receipts/abc.jpg"
	receiptDate := time.Now().UTC().AddDate(0, 0, -2)

	payload := map[string]any{
		"storeName":      "Bosch Servis",
		"totalAmount":    3400.00,
		"currency":       "TRY",
		"category":       "tamir",
		"receiptDate":    receiptDate.Format(time.RFC3339),
		"localImagePath": imagePath,
	}
	body, _ := json.Marshal(payload)

	rec := h.do(t, http.MethodPost, "/api/v1/receipts/confirm", "application/json", bytes.NewReader(body), true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}

	if len(h.repo.created) != 1 {
		t.Fatalf("repo created %d receipts, want 1", len(h.repo.created))
	}

	saved := h.repo.created[0]
	if saved.UserID != h.user.ID {
		t.Errorf("UserID = %v, want token'daki %v", saved.UserID, h.user.ID)
	}
	if saved.Category != store.CategoryRepair {
		t.Errorf("Category = %q, want repair", saved.Category)
	}
	if saved.LocalImagePath == nil || *saved.LocalImagePath != imagePath {
		t.Errorf("LocalImagePath = %v, want %q", saved.LocalImagePath, imagePath)
	}
}

func TestConfirm_RejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{
			name:    "unknown category",
			payload: map[string]any{"storeName": "X", "totalAmount": 10, "currency": "TRY", "category": "bilinmeyen", "receiptDate": time.Now().UTC().Format(time.RFC3339)},
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "negative amount",
			payload: map[string]any{"storeName": "X", "totalAmount": -5, "currency": "TRY", "category": "food", "receiptDate": time.Now().UTC().Format(time.RFC3339)},
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "malformed date",
			payload: map[string]any{"storeName": "X", "totalAmount": 10, "currency": "TRY", "category": "food", "receiptDate": "17.10.2026"},
			want:    http.StatusBadRequest,
		},
		{
			name:    "missing store name",
			payload: map[string]any{"totalAmount": 10, "currency": "TRY", "category": "food", "receiptDate": time.Now().UTC().Format(time.RFC3339)},
			want:    http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			body, _ := json.Marshal(tc.payload)

			rec := h.do(t, http.MethodPost, "/api/v1/receipts/confirm", "application/json", bytes.NewReader(body), true)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
			if len(h.repo.created) != 0 {
				t.Error("geçersiz veri veritabanına yazılmamalı")
			}
		})
	}
}

func TestConfirm_RequiresAuth(t *testing.T) {
	h := newHarness(t)
	body := strings.NewReader(`{}`)

	rec := h.do(t, http.MethodPost, "/api/v1/receipts/confirm", "application/json", body, false)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHealthEndpoints(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := h.do(t, http.MethodGet, path, "", nil, false)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
	}
}
