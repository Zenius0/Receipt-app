package receipts_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store"
)

type stubScanner struct {
	result ai.ScanResult
	err    error
	seen   ai.Image
}

func (s *stubScanner) Name() string { return "stub" }

func (s *stubScanner) Scan(_ context.Context, image ai.Image) (ai.ScanResult, error) {
	s.seen = image
	return s.result, s.err
}

type stubRepo struct {
	created []store.Receipt
	err     error
}

func (r *stubRepo) Create(_ context.Context, receipt store.Receipt) (store.Receipt, error) {
	if r.err != nil {
		return store.Receipt{}, r.err
	}
	receipt.CreatedAt = time.Now().UTC()
	r.created = append(r.created, receipt)
	return receipt, nil
}

func newService(t *testing.T, scanner ai.Scanner, repo store.ReceiptRepository) *receipts.Service {
	t.Helper()

	service, err := receipts.NewService(repo, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestService_Scan_PassesImageThrough(t *testing.T) {
	scanner := &stubScanner{result: ai.ScanResult{
		StoreName:   "Shell",
		TotalAmount: 500,
		Currency:    "TRY",
		Category:    store.CategoryFuel,
		ReceiptDate: time.Now().UTC(),
	}}
	service := newService(t, scanner, &stubRepo{})

	image := ai.Image{Data: []byte("bytes"), MimeType: "image/jpeg"}

	result, err := service.Scan(context.Background(), image)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.StoreName != "Shell" {
		t.Errorf("StoreName = %q, want Shell", result.StoreName)
	}
	if len(scanner.seen.Data) != len(image.Data) {
		t.Errorf("scanner received %d bytes, want %d", len(scanner.seen.Data), len(image.Data))
	}
}

func TestService_Confirm_NormalizesAndStores(t *testing.T) {
	repo := &stubRepo{}
	service := newService(t, &stubScanner{}, repo)

	userID := uuid.New()
	imagePath := "file:///var/mobile/.../receipt.jpg"
	receiptDate := time.Now().UTC().AddDate(0, 0, -3)

	receipt, err := service.Confirm(context.Background(), userID, receipts.ConfirmInput{
		StoreName:      "  Migros  ",
		TotalAmount:    1234.567,
		Currency:       "₺",
		Category:       "Yemek",
		ReceiptDate:    receiptDate,
		LocalImagePath: &imagePath,
	})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if receipt.StoreName != "Migros" {
		t.Errorf("StoreName = %q, want Migros (normalize edilmiş)", receipt.StoreName)
	}
	if receipt.TotalAmount != 1234.57 {
		t.Errorf("TotalAmount = %v, want 1234.57 (iki ondalığa yuvarlanmış)", receipt.TotalAmount)
	}
	if receipt.Currency != "TRY" {
		t.Errorf("Currency = %q, want TRY", receipt.Currency)
	}
	if receipt.Category != store.CategoryFood {
		t.Errorf("Category = %q, want food", receipt.Category)
	}
	if receipt.UserID != userID {
		t.Errorf("UserID = %v, want %v", receipt.UserID, userID)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo created %d receipts, want 1", len(repo.created))
	}
}

// Confirm başarısız olduğunda hiçbir şey yazılmamalıdır.
func TestService_Confirm_InvalidInputWritesNothing(t *testing.T) {
	repo := &stubRepo{}
	service := newService(t, &stubScanner{}, repo)

	tests := []struct {
		name string
		in   receipts.ConfirmInput
	}{
		{"empty store name", receipts.ConfirmInput{TotalAmount: 10, Currency: "TRY", Category: "food", ReceiptDate: time.Now().UTC()}},
		{"negative amount", receipts.ConfirmInput{StoreName: "X", TotalAmount: -1, Currency: "TRY", Category: "food", ReceiptDate: time.Now().UTC()}},
		{"bad currency", receipts.ConfirmInput{StoreName: "X", TotalAmount: 10, Currency: "AB", Category: "food", ReceiptDate: time.Now().UTC()}},
		{"unknown category", receipts.ConfirmInput{StoreName: "X", TotalAmount: 10, Currency: "TRY", Category: "bilinmeyen", ReceiptDate: time.Now().UTC()}},
		{"future date", receipts.ConfirmInput{StoreName: "X", TotalAmount: 10, Currency: "TRY", Category: "food", ReceiptDate: time.Now().UTC().AddDate(0, 0, 10)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Confirm(context.Background(), uuid.New(), tc.in)
			if !errors.Is(err, receipts.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}

	if len(repo.created) != 0 {
		t.Errorf("repo created %d receipts, want 0", len(repo.created))
	}
}

func TestService_Confirm_PropagatesRepositoryError(t *testing.T) {
	repo := &stubRepo{err: errors.New("connection reset")}
	service := newService(t, &stubScanner{}, repo)

	_, err := service.Confirm(context.Background(), uuid.New(), receipts.ConfirmInput{
		StoreName:   "X",
		TotalAmount: 10,
		Currency:    "TRY",
		Category:    "food",
		ReceiptDate: time.Now().UTC(),
	})

	if err == nil {
		t.Fatal("Confirm succeeded, want repository error")
	}
	if errors.Is(err, receipts.ErrInvalidInput) {
		t.Error("repository hatası istemci girdisi hatası olarak sınıflandırılmamalı")
	}
}

func TestNewService_RequiresDependencies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := receipts.NewService(nil, &stubScanner{}, logger); err == nil {
		t.Error("NewService accepted nil repository")
	}
	if _, err := receipts.NewService(&stubRepo{}, nil, logger); err == nil {
		t.Error("NewService accepted nil scanner")
	}
}
