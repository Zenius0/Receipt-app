package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/store"
)

func TestList_ReturnsUserScopedReceipts(t *testing.T) {
	h := newHarness(t)

	saved := []store.Receipt{
		{ID: uuid.New(), UserID: h.user.ID, StoreName: "Shell", TotalAmount: 100, Currency: "TRY", Category: store.CategoryFuel},
		{ID: uuid.New(), UserID: h.user.ID, StoreName: "Migros", TotalAmount: 250.50, Currency: "TRY", Category: store.CategoryGroceries},
	}
	h.repo.list = saved

	rec := h.do(t, http.MethodGet, "/api/v1/receipts", "", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Sorgu daima JWT'deki kullanıcıyla sınırlandırılmalı.
	if len(h.repo.listArgs) != 1 {
		t.Fatalf("repository calls = %d, want 1", len(h.repo.listArgs))
	}
	if got := h.repo.listArgs[0].userID; got != h.user.ID {
		t.Errorf("repository userID = %v, want token'daki %v", got, h.user.ID)
	}

	var response struct {
		Items      []store.Receipt `json:"items"`
		NextOffset *int            `json:"nextOffset"`
		Total      int             `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(response.Items) != 2 {
		t.Errorf("items = %d, want 2", len(response.Items))
	}
	if response.Items[0].UserID != h.user.ID {
		t.Errorf("item userID = %v, want %v", response.Items[0].UserID, h.user.ID)
	}
}

// Boş sonuç null değil boş dizi dönmeli; mobil istemci null ayrımıyla uğraşmasın.
func TestList_EmptyResultIsArray(t *testing.T) {
	h := newHarness(t)
	h.repo.list = nil

	rec := h.do(t, http.MethodGet, "/api/v1/receipts", "", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"items":[]`) {
		t.Errorf("body = %s, want items as empty array", body)
	}
}

func TestList_RequiresAuth(t *testing.T) {
	h := newHarness(t)

	rec := h.do(t, http.MethodGet, "/api/v1/receipts", "", nil, false)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(h.repo.listArgs) != 0 {
		t.Error("yetkisiz istek veritabanına ulaşmamalı")
	}
}

func TestList_QueryFilters(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		assert     func(t *testing.T, filter store.ReceiptFilter)
	}{
		{
			name:       "limit ve offset",
			query:      "?limit=10&offset=20",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, f store.ReceiptFilter) {
				if f.Limit != 10 || f.Offset != 20 {
					t.Errorf("limit/offset = %d/%d, want 10/20", f.Limit, f.Offset)
				}
			},
		},
		{
			name:       "limit max sınırına kırpılır",
			query:      "?limit=5000",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, f store.ReceiptFilter) {
				if f.Limit != store.MaxListLimit {
					t.Errorf("Limit = %d, want %d", f.Limit, store.MaxListLimit)
				}
			},
		},
		{
			name:       "tarih aralığı",
			query:      "?from=2026-10-01&to=2026-10-31",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, f store.ReceiptFilter) {
				if f.From == nil || f.To == nil {
					t.Fatalf("from/to = %v/%v, want non-nil", f.From, f.To)
				}
				if f.From.Day() != 1 || f.To.Day() != 31 {
					t.Errorf("from/to = %v/%v, want 1 ve 31", f.From, f.To)
				}
			},
		},
		{
			name:       "kategori Türkçe alias",
			query:      "?category=Yak%C4%B1t",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, f store.ReceiptFilter) {
				if f.Category != store.CategoryFuel {
					t.Errorf("Category = %q, want fuel", f.Category)
				}
			},
		},
		{
			name:       "geçersiz kategori",
			query:      "?category=bilinmeyen",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "geçersiz limit",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "geçersiz tarih",
			query:      "?from=dun",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "tarih aralığı ters",
			query:      "?from=2026-10-31&to=2026-10-01",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(t, http.MethodGet, "/api/v1/receipts"+tc.query, "", nil, true)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.assert != nil {
				if len(h.repo.listArgs) != 1 {
					t.Fatalf("repository calls = %d, want 1", len(h.repo.listArgs))
				}
				tc.assert(t, h.repo.listArgs[0].filter)
			}
		})
	}
}
