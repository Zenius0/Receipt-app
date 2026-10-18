package receipts_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store"
)

func TestList_Pagination(t *testing.T) {
	userID := uuid.New()

	tests := []struct {
		name       string
		stored     int
		limit      int
		offset     int
		wantCount  int
		wantNext   bool
		wantOffset int
	}{
		{"ilk sayfa dolu, devam var", 10, 5, 0, 5, true, 5},
		{"tam sayfa ama veri bitti", 3, 5, 0, 3, false, 0},
		{"ikinci sayfa kısmi", 10, 5, 5, 5, true, 10},
		{"sonda dolu sayfa yok", 10, 5, 10, 0, false, 0},
		{"limit verilmemiş", 3, 0, 0, 3, false, 0},
		// İstemci limiti bilerek aşar (sabit değer gönderen hatalı istemci):
		// sunucu sessizce MaxListLimit'e kırpmalı, hata dönmemeli.
		{"limit max sınırını aşıyor", store.MaxListLimit + 50, store.MaxListLimit + 50, 0, store.MaxListLimit, true, store.MaxListLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]store.Receipt, tc.stored)
			for i := range items {
				items[i] = store.Receipt{ID: uuid.New(), UserID: userID}
			}

			repo := &stubRepo{list: items}
			service := newService(t, &stubScanner{}, repo)

			result, err := service.List(context.Background(), userID, receipts.ListInput{
				Limit: tc.limit, Offset: tc.offset,
			})
			if err != nil {
				t.Fatalf("List: %v", err)
			}

			if len(result.Items) != tc.wantCount {
				t.Errorf("items = %d, want %d", len(result.Items), tc.wantCount)
			}

			if tc.wantNext {
				if result.NextOffset == nil {
					t.Fatal("NextOffset = nil, want a next page")
				}
				if *result.NextOffset != tc.wantOffset {
					t.Errorf("NextOffset = %d, want %d", *result.NextOffset, tc.wantOffset)
				}
			} else if result.NextOffset != nil {
				t.Errorf("NextOffset = %d, want nil (liste bitti)", *result.NextOffset)
			}
		})
	}
}

// Sayfalama istemciden gelen değerleri repository'ye aynen taşımalı.
func TestList_PassesFilterToRepository(t *testing.T) {
	userID := uuid.New()
	from := time.Now().UTC().AddDate(0, 0, -10)
	to := time.Now().UTC().AddDate(0, 0, -5)

	repo := &stubRepo{}
	service := newService(t, &stubScanner{}, repo)

	_, err := service.List(context.Background(), userID, receipts.ListInput{
		Category: "Yakıt", // Türkçe alias kanonikleştirilmeli
		From:     &from,
		To:       &to,
		Limit:    25,
		Offset:   50,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(repo.listArgs) != 1 {
		t.Fatalf("repository calls = %d, want 1", len(repo.listArgs))
	}

	call := repo.listArgs[0]
	if call.userID != userID {
		t.Errorf("userID = %v, want %v", call.userID, userID)
	}
	if call.filter.Category != store.CategoryFuel {
		t.Errorf("Category = %q, want fuel (Türkçe alias normalize edilmeli)", call.filter.Category)
	}
	if call.filter.Limit != 25 || call.filter.Offset != 50 {
		t.Errorf("limit/offset = %d/%d, want 25/50", call.filter.Limit, call.filter.Offset)
	}
}

func TestList_RejectsUnknownCategory(t *testing.T) {
	repo := &stubRepo{}
	service := newService(t, &stubScanner{}, repo)

	_, err := service.List(context.Background(), uuid.New(), receipts.ListInput{Category: "bilinmeyen"})
	if !errors.Is(err, receipts.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	if len(repo.listArgs) != 0 {
		t.Error("geçersiz filtre için sorgu çalıştırılmamalı")
	}
}

// Negatif offset sessizce sıfırlanmalı (sürükle-bırak liste kaydırması sırasında
// arka plan yanlışlıkla -1 gönderirse sunucu hata vermemeli).
func TestList_NormalizesNegativeOffset(t *testing.T) {
	repo := &stubRepo{}
	service := newService(t, &stubScanner{}, repo)

	if _, err := service.List(context.Background(), uuid.New(), receipts.ListInput{Offset: -5}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := repo.listArgs[0].filter.Offset; got != 0 {
		t.Errorf("Offset = %d, want 0", got)
	}
}

func TestList_PropagatesRepositoryError(t *testing.T) {
	repo := &stubRepo{listErr: errors.New("connection reset")}
	service := newService(t, &stubScanner{}, repo)

	_, err := service.List(context.Background(), uuid.New(), receipts.ListInput{})
	if err == nil {
		t.Fatal("List succeeded, want repository error")
	}
	if errors.Is(err, receipts.ErrInvalidInput) {
		t.Error("veritabanı hatası istemci girdisi hatası olarak sınıflandırılmamalı")
	}
}

func TestNewService_RequiresRepository(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := receipts.NewService(nil, &stubScanner{}, logger); err == nil {
		t.Error("NewService accepted nil repository")
	}
}
