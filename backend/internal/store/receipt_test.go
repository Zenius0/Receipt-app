package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/store"
)

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		input string
		want  store.Category
	}{
		{"fuel", store.CategoryFuel},
		{"FUEL", store.CategoryFuel},
		{"Yakıt", store.CategoryFuel},
		{"  yakit  ", store.CategoryFuel},
		{"Akaryakıt", store.CategoryFuel},
		{"Yemek", store.CategoryFood},
		{"Restoran", store.CategoryFood},
		{"Yazılım", store.CategorySoftware},
		{"SaaS", store.CategorySoftware},
		{"sağlık", store.CategoryHealth},
		{"Eğlence", store.CategoryEntertainment},
		{"giyim", store.CategoryClothing},
		{"fatura", store.CategoryUtilities},
		{"diğer", store.CategoryOther},
		{"Other", store.CategoryOther},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := store.NormalizeCategory(tc.input)
			if err != nil {
				t.Fatalf("NormalizeCategory(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeCategory(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeCategory_RejectsUnknown(t *testing.T) {
	for _, input := range []string{"", "   ", "bilinmeyen", "12345"} {
		if _, err := store.NormalizeCategory(input); !errors.Is(err, store.ErrInvalidCategory) {
			t.Errorf("NormalizeCategory(%q) error = %v, want ErrInvalidCategory", input, err)
		}
	}
}

func TestNormalizeCurrency(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"TRY", "TRY"},
		{"try", "TRY"},
		{"₺", "TRY"},
		{"TL", "TRY"},
		{"$", "USD"},
		{"usd", "USD"},
		{"€", "EUR"},
		{"eur", "EUR"},
		{"gbp", "GBP"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := store.NormalizeCurrency(tc.input)
			if err != nil {
				t.Fatalf("NormalizeCurrency(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeCurrency(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeCurrency_RejectsMalformed(t *testing.T) {
	for _, input := range []string{"", "TR", "TRYTRY", "12A"} {
		if _, err := store.NormalizeCurrency(input); !errors.Is(err, store.ErrInvalidCurrency) {
			t.Errorf("NormalizeCurrency(%q) error = %v, want ErrInvalidCurrency", input, err)
		}
	}
}

func TestNormalizeAmount(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0.00"},
		{10, "10.00"},
		{1234.567, "1234.57"},
		{0.005, "0.01"},
		{19.99, "19.99"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got, err := store.NormalizeAmount(tc.input)
			if err != nil {
				t.Fatalf("NormalizeAmount(%v): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeAmount(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	for _, input := range []float64{-1, 1e12} {
		if _, err := store.NormalizeAmount(input); !errors.Is(err, store.ErrInvalidAmount) {
			t.Errorf("NormalizeAmount(%v) error = %v, want ErrInvalidAmount", input, err)
		}
	}
}

func TestNormalizeStoreName(t *testing.T) {
	got, err := store.NormalizeStoreName("  Shell   Petrol \n A.Ş. ")
	if err != nil {
		t.Fatalf("NormalizeStoreName: %v", err)
	}
	if got != "Shell Petrol A.Ş." {
		t.Errorf("got %q, want %q", got, "Shell Petrol A.Ş.")
	}

	if _, err := store.NormalizeStoreName("   "); !errors.Is(err, store.ErrInvalidStore) {
		t.Errorf("blank name error = %v, want ErrInvalidStore", err)
	}

	if _, err := store.NormalizeStoreName(strings.Repeat("a", 201)); !errors.Is(err, store.ErrInvalidStore) {
		t.Errorf("too long name error = %v, want ErrInvalidStore", err)
	}
}

func TestNormalizeReceiptDate(t *testing.T) {
	got, err := store.NormalizeReceiptDate(time.Now().AddDate(0, 0, -1).In(time.FixedZone("TRT", 3*3600)))
	if err != nil {
		t.Fatalf("NormalizeReceiptDate: %v", err)
	}
	if got.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", got.Location())
	}

	if _, err := store.NormalizeReceiptDate(time.Now().AddDate(0, 0, 3)); !errors.Is(err, store.ErrInvalidDate) {
		t.Error("gelecek tarih kabul edildi")
	}
	if _, err := store.NormalizeReceiptDate(time.Now().AddDate(-20, 0, 0)); !errors.Is(err, store.ErrInvalidDate) {
		t.Error("20 yıl öncesi tarih kabul edildi")
	}
	if _, err := store.NormalizeReceiptDate(time.Time{}); !errors.Is(err, store.ErrInvalidDate) {
		t.Error("sıfır tarih kabul edildi")
	}
}

func TestNewReceipt(t *testing.T) {
	path := "  file:///receipts/x.jpg  "
	userID := uuid.New()

	receipt, err := store.NewReceipt(userID, " Migros ", 89.999, "₺", "Market", time.Now().AddDate(0, 0, -1), &path)
	if err != nil {
		t.Fatalf("NewReceipt: %v", err)
	}

	if receipt.ID == uuid.Nil {
		t.Error("id was not generated")
	}
	if receipt.StoreName != "Migros" {
		t.Errorf("StoreName = %q", receipt.StoreName)
	}
	if receipt.TotalAmount != 90.00 {
		t.Errorf("TotalAmount = %v, want 90.00", receipt.TotalAmount)
	}
	if receipt.Currency != "TRY" {
		t.Errorf("Currency = %q", receipt.Currency)
	}
	if receipt.Category != store.CategoryGroceries {
		t.Errorf("Category = %q", receipt.Category)
	}
	if receipt.LocalImagePath == nil || *receipt.LocalImagePath != "file:///receipts/x.jpg" {
		t.Errorf("LocalImagePath = %v (trim edilmedi)", receipt.LocalImagePath)
	}
}

// Görsel yolu boş gönderilirse satır NULL olarak yazılmalıdır.
func TestNewReceipt_EmptyImagePathBecomesNil(t *testing.T) {
	blank := "   "

	receipt, err := store.NewReceipt(uuid.New(), "X", 10, "TRY", "food", time.Now(), &blank)
	if err != nil {
		t.Fatalf("NewReceipt: %v", err)
	}
	if receipt.LocalImagePath != nil {
		t.Errorf("LocalImagePath = %v, want nil", receipt.LocalImagePath)
	}
}

func TestNewReceipt_RequiresUser(t *testing.T) {
	_, err := store.NewReceipt(uuid.Nil, "X", 10, "TRY", "food", time.Now(), nil)
	if err == nil {
		t.Error("NewReceipt accepted nil user id")
	}
}
