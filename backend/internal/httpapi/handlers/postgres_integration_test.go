package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/httpapi"
	"github.com/selim/fatura-backend/internal/platform/db"
	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store"
	"github.com/selim/fatura-backend/internal/store/pgstore"
)

// TestReceiptFlow_Postgres, yalnızca DATABASE_URL verildiğinde çalışır.
// Fiş akışının gerçek PostgreSQL'e karşı uçtan uca çalıştığını doğrular:
// migration çalıştırma, INSERT, RETURNING ve kısıtlar.
//
//	go test ./internal/httpapi/handlers -run Postgres -v
func TestReceiptFlow_Postgres(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set; skipping postgres integration test")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("db.NewPool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	userID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, provider, provider_user_id)
		VALUES ($1, $2, 'google', $3)
		ON CONFLICT (provider, provider_user_id) DO UPDATE SET email = EXCLUDED.email`,
		userID, fmt.Sprintf("pg-test-%s@example.com", userID), userID.String())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	issuer, err := auth.NewTokenIssuer([]byte("postgres-integration-test-secret-key-32b"), time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	token, _, err := issuer.Issue(store.User{ID: userID, Email: "pg-test@example.com", Provider: "google"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	scanner := &stubScanner{result: ai.ScanResult{
		StoreName:   "Migros",
		TotalAmount: 349.90,
		Currency:    "TRY",
		Category:    store.CategoryGroceries,
		ReceiptDate: time.Now().UTC().AddDate(0, 0, -1),
	}}
	repo := pgstore.NewReceiptRepository(pool)

	service, err := receipts.NewService(repo, scanner, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server := httpapi.NewRouter(httpapi.Dependencies{
		Logger:         logger,
		TokenIssuer:    issuer,
		ReceiptService: service,
		MaxScanImage:   ai.MaxImageBytes,
	})

	post := func(path string, body []byte, contentType string) *httptest.ResponseRecorder {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, "http://api.test"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Authorization", "Bearer "+token)

		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	// 1) Scan
	scanBody, _ := json.Marshal(map[string]string{
		"imageBase64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
	})
	scanRec := post("/api/v1/receipts/scan", scanBody, "application/json")
	if scanRec.Code != http.StatusOK {
		t.Fatalf("scan status = %d, body = %s", scanRec.Code, scanRec.Body.String())
	}

	// 2) Confirm
	imagePath := "file:///data/user/0/com.fatura/receipts/x.jpg"
	confirmBody, _ := json.Marshal(map[string]any{
		"storeName":      "Migros",
		"totalAmount":    349.90,
		"currency":       "TRY",
		"category":       "groceries",
		"receiptDate":    scanner.result.ReceiptDate.Format(time.RFC3339),
		"localImagePath": imagePath,
	})
	confirmRec := post("/api/v1/receipts/confirm", confirmBody, "application/json")
	if confirmRec.Code != http.StatusCreated {
		t.Fatalf("confirm status = %d, body = %s", confirmRec.Code, confirmRec.Body.String())
	}

	var created store.Receipt
	if err := json.Unmarshal(confirmRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("created receipt has no id")
	}
	if created.UserID != userID {
		t.Errorf("UserID = %v, want %v", created.UserID, userID)
	}

	// 3) Satır gerçekten duruyor mu?
	var (
		storedPath *string
		amount     float64
		category   string
	)
	err = pool.QueryRow(ctx, `
		SELECT local_image_path, total_amount, category
		FROM receipts WHERE id = $1`, created.ID).
		Scan(&storedPath, &amount, &category)
	if err != nil {
		t.Fatalf("read back receipt: %v", err)
	}
	if amount != 349.90 {
		t.Errorf("total_amount = %v, want 349.90", amount)
	}
	if category != "groceries" {
		t.Errorf("category = %q, want groceries", category)
	}
	if storedPath == nil || *storedPath != imagePath {
		t.Errorf("local_image_path = %v, want %q", storedPath, imagePath)
	}

	// 4) CHECK kısıtları gerçekten çalışıyor mu?
	if _, err := pool.Exec(ctx, `
		INSERT INTO receipts (user_id, store_name, total_amount, currency, category, receipt_date)
		VALUES ($1, 'X', 10, 'TRY', 'gecersiz-kategori', now())`, userID); err == nil {
		t.Error("geçersiz kategori CHECK kısıtı uygulanmadı")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO receipts (user_id, store_name, total_amount, currency, category, receipt_date)
		VALUES ($1, 'X', 10, 'TRYLARI', 'food', now())`, userID); err == nil {
		t.Error("geçersiz para birimi CHECK kısıtı uygulanmadı")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO receipts (user_id, store_name, total_amount, currency, category, receipt_date)
		VALUES ($1, 'X', 10, 'TRY', 'food', now() + INTERVAL '30 days')`, userID); err == nil {
		t.Error("gelecek tarih CHECK kısıtı uygulanmadı")
	}

	// Temizlik
	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	t.Log("postgres integration test passed")
}
