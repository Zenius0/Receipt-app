package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// TestReceiptList_Postgres, liste ucunu gerçek PostgreSQL'e karşı doğrular:
// kullanıcı izolasyonu, kategori ve tarih filtresi, sayfalama sırası.
//
//	go test ./internal/httpapi/handlers -run Postgres -v
func TestReceiptList_Postgres(t *testing.T) {
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

	// İki kullanıcı: biri diğerinin fişlerini GÖRMEMELİ.
	owner := uuid.New()
	stranger := uuid.New()
	for _, id := range []uuid.UUID{owner, stranger} {
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, provider, provider_user_id)
			VALUES ($1, $2, 'google', $3)
			ON CONFLICT (provider, provider_user_id) DO UPDATE SET email = EXCLUDED.email`,
			id, "list-test-"+id.String()+"@example.com", id.String())
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{owner, stranger})
	})

	repo := pgstore.NewReceiptRepository(pool)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	fixtures := []struct {
		owner    uuid.UUID
		store    string
		amount   float64
		category store.Category
		age      int
		// createdAt, test verisinin sıralamasını belirlenir kılmak için
		// açıkça verilir. Gerçekte created_at saniye altı çakışabilir;
		// o durumda id DESC devreye girer ve sayfalama kararlı kalır.
		createdAt time.Time
	}{
		{owner, "Shell", 100, store.CategoryFuel, 1, time.Now().UTC().Add(-1 * time.Hour)},
		{owner, "Migros", 200, store.CategoryGroceries, 2, time.Now().UTC().Add(-2 * time.Hour)},
		{owner, "Bosch", 300, store.CategoryRepair, 3, time.Now().UTC().Add(-3 * time.Hour)},
		{stranger, "BAKI Tesisat", 999, store.CategoryRepair, 1, time.Now().UTC()},
	}
	for _, f := range fixtures {
		if _, err := pool.Exec(ctx, `
			INSERT INTO receipts (user_id, store_name, total_amount, currency, category, receipt_date, local_image_path, created_at)
			VALUES ($1, $2, $3, 'TRY', $4, $5, NULL, $6)`,
			f.owner, f.store, f.amount, string(f.category), today.AddDate(0, 0, -f.age), f.createdAt); err != nil {
			t.Fatalf("seed receipt: %v", err)
		}
	}

	// 1) Kullanıcı izolasyonu
	all, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("owner fiş sayısı = %d, want 3 (yabancının fişi sızmamalı)", len(all))
	}
	for _, r := range all {
		if r.UserID != owner {
			t.Errorf("fiş %s kullanıcısı = %v, want %v", r.ID, r.UserID, owner)
		}
	}

	strangerList, err := repo.ListByUser(ctx, stranger, store.ReceiptFilter{})
	if err != nil {
		t.Fatalf("ListByUser(stranger): %v", err)
	}
	if len(strangerList) != 1 {
		t.Errorf("yabancı fiş sayısı = %d, want 1", len(strangerList))
	}

	// 2) Kategori filtresi (Türkçe alias normalize edilmeli)
	fuel, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{Category: store.Category("Yakıt")})
	if err != nil {
		t.Fatalf("ListByUser(category): %v", err)
	}
	if len(fuel) != 1 || fuel[0].StoreName != "Shell" {
		t.Errorf("kategori filtresi = %d kayıt, want 1 (Shell)", len(fuel))
	}

	// 3) Tarih aralığı — "son 2 gün" Shell + Migros'u kapsamalı, Bosch'u değil
	from := today.AddDate(0, 0, -2)
	byDate, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{From: &from, To: &today})
	if err != nil {
		t.Fatalf("ListByUser(dates): %v", err)
	}
	if len(byDate) != 2 {
		t.Errorf("tarih filtresi = %d kayıt, want 2 (Shell + Migros)", len(byDate))
	}

	// 4) Sayfalama ve sıralama — en yeni önce
	paged, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListByUser(limit): %v", err)
	}
	if len(paged) != 2 {
		t.Fatalf("limit uygulanmadı: %d kayıt", len(paged))
	}
	if paged[0].StoreName != "Shell" || paged[1].StoreName != "Migros" {
		t.Errorf("sıralama = %s, %s; want Shell, Migros (en yeni önce)", paged[0].StoreName, paged[1].StoreName)
	}

	// 5) Limit tavanı — istemci 5000 ister, sunucu 100 döner
	capped, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{Limit: 5000})
	if err != nil {
		t.Fatalf("ListByUser(cap): %v", err)
	}
	if len(capped) > store.MaxListLimit {
		t.Errorf("limit tavanı uygulanmadı: %d", len(capped))
	}

	// 6) SQL injection denemesi — üç katmanlı savunmanın EN DIŞTAKI katmanı
	// işe yarıyor: kategori beyaz listeden geçemediği için sorgu hiç
	// çalıştırılmıyor. Parametreli sorgu ise ikinci savunmadır.
	injected, err := repo.ListByUser(ctx, owner, store.ReceiptFilter{
		Category: store.Category("fuel'; DROP TABLE receipts;--"),
	})
	if !errors.Is(err, store.ErrInvalidCategory) {
		t.Errorf("injection girdisi hata vermeliydi, err = %v / %d kayıt", err, len(injected))
	}

	// Tablo sağlam mı?
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM receipts`).Scan(&count); err != nil {
		t.Fatalf("tablo düştü: %v", err)
	}
	if count < 4 {
		t.Errorf("receipts satır sayısı = %d, want >= 4", count)
	}

	// 7) Yaklaşık toplam: 3 fiş var, ilk sayfa 2 → nextOffset işaret etmeli
	service, err := receipts.NewService(repo, &stubScanner{}, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	page, err := service.List(ctx, owner, receipts.ListInput{Limit: 2})
	if err != nil {
		t.Fatalf("service.List: %v", err)
	}
	if page.NextOffset == nil || *page.NextOffset != 2 {
		t.Errorf("NextOffset = %v, want 2", page.NextOffset)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2 (bu sayfadaki kayıt sayısı)", page.Total)
	}

	t.Log("postgres list test passed")
}
