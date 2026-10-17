package ai

import (
	"errors"
	"testing"
	"time"

	"github.com/selim/fatura-backend/internal/store"
)

func TestParseScanResponse_Valid(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	payload := `{
		"isReceipt": true,
		"storeName": "  Shell   Petrol  ",
		"totalAmount": 1250.75,
		"currency": "TRY",
		"category": "fuel",
		"receiptDate": "` + yesterday + `",
		"taxNumber": "1234567890"
	}`

	result, err := ParseScanResponse(payload)
	if err != nil {
		t.Fatalf("ParseScanResponse: %v", err)
	}

	if result.StoreName != "Shell Petrol" {
		t.Errorf("StoreName = %q, want %q", result.StoreName, "Shell Petrol")
	}
	if result.TotalAmount != 1250.75 {
		t.Errorf("TotalAmount = %v, want 1250.75", result.TotalAmount)
	}
	if result.Currency != "TRY" {
		t.Errorf("Currency = %q, want TRY", result.Currency)
	}
	if result.Category != store.CategoryFuel {
		t.Errorf("Category = %q, want fuel", result.Category)
	}
	if result.ReceiptDate.Location() != time.UTC {
		t.Errorf("ReceiptDate location = %v, want UTC", result.ReceiptDate.Location())
	}
	if result.ReceiptDate.Hour() != 12 {
		t.Errorf("ReceiptDate hour = %d, want 12 (gün kaymasını önlemek için öğlen)", result.ReceiptDate.Hour())
	}
	if result.TaxNumber == nil || *result.TaxNumber != "1234567890" {
		t.Errorf("TaxNumber = %v, want 1234567890", result.TaxNumber)
	}
}

// Model, şema adlarında alt çizgi veya farklı büyüklük kullanabilir;
// ayrıştırıcı bunu tolere etmelidir.
func TestParseScanResponse_ToleratesKeyAndValueVariants(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, -2).Format("02.01.2006")

	payload := `{
		"Is_Receipt": true,
		"store_name": "Migros",
		"total_amount": "1.250,75",
		"currency": "₺",
		"category": "Yakıt",
		"receipt_date": "` + date + `"
	}`

	result, err := ParseScanResponse(payload)
	if err != nil {
		t.Fatalf("ParseScanResponse: %v", err)
	}

	if result.TotalAmount != 1250.75 {
		t.Errorf("TotalAmount = %v, want 1250.75 (Avrupa biçimi metin)", result.TotalAmount)
	}
	if result.Currency != "TRY" {
		t.Errorf("Currency = %q, want TRY", result.Currency)
	}
	if result.Category != store.CategoryFuel {
		t.Errorf("Category = %q, want fuel", result.Category)
	}
}

func TestParseScanResponse_StripsCodeFence(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")

	payload := "```json\n" + `{"isReceipt":true,"storeName":"A101","totalAmount":10,"currency":"TRY","category":"other","receiptDate":"` + date + `"}` + "\n```"

	result, err := ParseScanResponse(payload)
	if err != nil {
		t.Fatalf("ParseScanResponse: %v", err)
	}
	if result.StoreName != "A101" {
		t.Errorf("StoreName = %q, want A101", result.StoreName)
	}
}

func TestParseScanResponse_NotAReceipt(t *testing.T) {
	payload := `{"isReceipt": false, "reason": "Bu görsel bir manzara fotoğrafı."}`

	_, err := ParseScanResponse(payload)
	if !errors.Is(err, ErrNotReceipt) {
		t.Fatalf("error = %v, want ErrNotReceipt", err)
	}

	var notReceipt *NotReceiptError
	if !errors.As(err, &notReceipt) {
		t.Fatalf("error = %T, want *NotReceiptError", err)
	}
	if notReceipt.Reason != "Bu görsel bir manzara fotoğrafı." {
		t.Errorf("Reason = %q, want model açıklaması", notReceipt.Reason)
	}
}

func TestParseScanResponse_NotAReceipt_DefaultReason(t *testing.T) {
	_, err := ParseScanResponse(`{"isReceipt": false}`)
	if !errors.Is(err, ErrNotReceipt) {
		t.Fatalf("error = %v, want ErrNotReceipt", err)
	}

	var notReceipt *NotReceiptError
	if !errors.As(err, &notReceipt) {
		t.Fatalf("error = %T, want *NotReceiptError", err)
	}
	if notReceipt.Reason == "" {
		t.Error("Reason must not be empty; kullanıcıya gösterilecek bir açıklama gerekir")
	}
}

func TestParseScanResponse_Incomplete(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"missing amount", `{"isReceipt": true, "storeName": "X", "currency": "TRY", "category": "food", "receiptDate": "2026-01-05"}`},
		{"missing store name", `{"isReceipt": true, "totalAmount": 10, "currency": "TRY", "category": "food", "receiptDate": "2026-01-05"}`},
		{"unknown category", `{"isReceipt": true, "storeName": "X", "totalAmount": 10, "currency": "TRY", "category": "bilinmeyen", "receiptDate": "2026-01-05"}`},
		{"unparsable date", `{"isReceipt": true, "storeName": "X", "totalAmount": 10, "currency": "TRY", "category": "food", "receiptDate": "geçen salı"}`},
		{"negative amount", `{"isReceipt": true, "storeName": "X", "totalAmount": -5, "currency": "TRY", "category": "food", "receiptDate": "2026-01-05"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseScanResponse(tc.payload)
			if !errors.Is(err, ErrIncompleteReceipt) {
				t.Fatalf("error = %v, want ErrIncompleteReceipt", err)
			}
		})
	}
}

func TestParseScanResponse_FutureDateRejected(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
	payload := `{"isReceipt": true, "storeName": "X", "totalAmount": 10, "currency": "TRY", "category": "food", "receiptDate": "` + future + `"}`

	_, err := ParseScanResponse(payload)
	if !errors.Is(err, ErrIncompleteReceipt) {
		t.Fatalf("error = %v, want ErrIncompleteReceipt", err)
	}
}

func TestParseScanResponse_Malformed(t *testing.T) {
	for _, payload := range []string{"", "not json", `{"isReceipt": true, "totalAmount": }`} {
		if _, err := ParseScanResponse(payload); err == nil {
			t.Errorf("ParseScanResponse(%q) succeeded, want error", payload)
		}
	}
}

// Prompt şemasındaki kategori enum'u ile veritabanı CHECK kısıtı ayrışmamalıdır.
func TestResponseSchema_CategoriesMatchStore(t *testing.T) {
	properties, ok := ResponseSchema()["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}

	category, ok := properties["category"].(map[string]any)
	if !ok {
		t.Fatal("schema has no category property")
	}

	enum, ok := category["enum"].([]string)
	if !ok {
		t.Fatalf("category enum has type %T, want []string", category["enum"])
	}

	want := store.AllCategories()
	if len(enum) != len(want) {
		t.Fatalf("enum has %d entries, want %d", len(enum), len(want))
	}
	for i, c := range want {
		if enum[i] != string(c) {
			t.Errorf("enum[%d] = %q, want %q", i, enum[i], string(c))
		}
	}
}
