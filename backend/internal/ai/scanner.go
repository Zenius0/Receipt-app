package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/selim/fatura-backend/internal/store"
)

// Scanner, fiş görselini yapay zekâya gönderip yapılandırılmış veri çıkaran
// sağlayıcı bağımsız arayüzdür. Gemini ve OpenAI bu arayüzü uygular;
// handler'lar somut sağlayıcıyı bilmez.
type Scanner interface {
	// Name, log ve hata mesajları için sağlayıcı adı.
	Name() string
	// Scan, görseli analiz eder. Görsel geçerli bir fiş değilse
	// ErrNotReceipt hatası döner.
	Scan(ctx context.Context, image Image) (ScanResult, error)
}

var (
	// ErrNotConfigured, sağlayıcı için API anahtarı eksik demektir.
	ErrNotConfigured = errors.New("ai: scanner is not configured")
	// ErrNotReceipt, görselde okunabilir bir fiş bulunamadı demektir.
	ErrNotReceipt = errors.New("ai: image is not a valid receipt")
	// ErrIncompleteReceipt, fiş geçerli ama zorunlu alanlardan biri okunamadı.
	ErrIncompleteReceipt = errors.New("ai: receipt data is incomplete")
	// ErrMalformedResponse, sağlayıcının döndürdüğü yanıt ayrıştırılamadı.
	ErrMalformedResponse = errors.New("ai: provider returned an unusable response")
	// ErrUpstream, sağlayıcıya ulaşılamadı veya 5xx döndü.
	ErrUpstream = errors.New("ai: provider request failed")
	// ErrRateLimited, sağlayıcı kota sınırına takıldı.
	ErrRateLimited = errors.New("ai: provider rate limit exceeded")
	// ErrUpstreamAuth, API anahtarı reddedildi.
	ErrUpstreamAuth = errors.New("ai: provider rejected the api key")
)

// NotReceiptError, modelin "bu bir fiş değil" açıklamasını taşır. Handler bu
// metni istemciye olduğu gibi iletir.
type NotReceiptError struct {
	Reason string
}

func (e *NotReceiptError) Error() string {
	return "ai: image is not a valid receipt: " + e.Reason
}

func (e *NotReceiptError) Unwrap() error { return ErrNotReceipt }

// IncompleteReceiptError, fişin geçerli olduğu ama bir alanın okunamadığı
// durumunda hangi alanın eksik olduğunu taşır.
type IncompleteReceiptError struct {
	Detail string
}

func (e *IncompleteReceiptError) Error() string {
	return "ai: receipt data is incomplete: " + e.Detail
}

func (e *IncompleteReceiptError) Unwrap() error { return ErrIncompleteReceipt }

// ScanResult, görselden çıkarılan ve istemciye onay için gösterilen veridir.
// Bu aşamada henüz veritabanına yazılmaz.
type ScanResult struct {
	StoreName   string
	TotalAmount float64
	Currency    string
	Category    store.Category
	ReceiptDate time.Time
	TaxNumber   *string
}

// receiptPrompt, modele verilen tek sistem görevidir. Yanıt şekli
// response_format içindeki JSON Schema ile zorlanır; buradaki metin alanların
// anlamını ve normalizasyon kurallarını tanımlar.
const receiptPrompt = `Sen bir fiş/fatura belge doğrulama ve veri çıkarma servisisin.

Sana bir görsel veriliyor. İki görevin var:

1) GEÇERLİLİK KONTROLÜ: Görsel gerçekten satış faturası, fiş, e-slip, kasa
   makbuzu veya benzeri bir ödeme belgesi mi? Değilse isReceipt=false döndür
   ve reason alanına kullanıcıya gösterilebilecek kısa bir Türkçe cümle yaz
   (örn. "Bu görsel bir fiş değil", "Fiş okunamıyor, daha net bir fotoğraf çekin").
   Bir fotoğrafın kendisi, boş kağıt, ekran görüntüsü, banka ekstresi veya
   ürün kutusu fiş sayılmaz.

2) ALAN ÇIKARMA: Fiş geçerliyse aşağıdaki alanları doldur.

Kurallar:
- storeName: fiş üzerinde yazılı işletme adı. Kendi kelimelerinle uydurma.
- totalAmount: fişteki ÖDENECEK TOPLAM / GENEL TOPLAM / TOPLAM DEĞER.
  Kuruş haneleri iki basamak olacak şekilde yaz. KDV dahil toplamı kullan;
  ara toplam veya KDV tutarını kullanma.
- currency: yalnızca ISO 4217 üç harfli kodu (TRY, USD, EUR, GBP ...).
  Fişteki sembolü (₺, $, €) veya adını değil kodu yaz.
- category: yalnızca şemada verilen enum listesinden tek bir değer seç.
- receiptDate: fiş üzerindeki tarih, YYYY-MM-DD biçiminde. Belgede tarih
  yoksa uydurma: isReceipt=false döndür.
- taxNumber: fişte vergi/TC kimlik numarası varsa, yoksa boş bırak.

Belge Türkçe, İngilizce veya başka bir dilde olsa da değerleri bu kurallara
göre normalize et. Yalnızca istenen JSON nesnesini döndür, açıklama yazma.`

// receiptResponseSchema, sağlayıcıya verilen JSON Schema'dır. Kategori enum'u
// store.AllCategories()'ten üretilir; böylece prompt, uygulama kodu ve veritabanı
// CHECK kısıtı tek bir kaynaktan beslenir.
func receiptResponseSchema() map[string]any {
	categories := make([]string, 0, len(store.AllCategories()))
	for _, c := range store.AllCategories() {
		categories = append(categories, string(c))
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"isReceipt": map[string]any{
				"type":        "boolean",
				"description": "Görsel geçerli bir fiş/fatura belgesi mi?",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Geçersizse kısa Türkçe açıklama; geçerliyse boş bırakılabilir.",
			},
			"storeName":   map[string]any{"type": "string", "description": "İşletme adı."},
			"totalAmount": map[string]any{"type": "number", "description": "Toplam tutar, iki ondalık."},
			"currency":    map[string]any{"type": "string", "description": "ISO 4217 üç harfli para birimi kodu."},
			"category":    map[string]any{"type": "string", "enum": categories},
			"receiptDate": map[string]any{"type": "string", "description": "Fiş tarihi, YYYY-MM-DD."},
			"taxNumber":   map[string]any{"type": "string", "description": "Vergi/TC kimlik numarası, yoksa boş."},
		},
		"required":             []string{"isReceipt"},
		"additionalProperties": false,
	}
}

// Prompt, Scanner uygulamalarının ortak kullanımı için dışa açıktır.
func Prompt() string { return receiptPrompt }

// ResponseSchema, Scanner uygulamalarının ortak kullanımı için dışa açıktır.
func ResponseSchema() map[string]any { return receiptResponseSchema() }

// ParseScanResponse, sağlayıcının döndürdüğü metni ayrıştırır, doğrular ve
// ScanResult'a çevirir. Her iki sağlayıcı da ortak doğrulamadan geçtiği için
// "AI ne döndürürse döndürsün" veritabanına yazılamaz.
//
// Toleranslı ayrıştırma: model alan adlarında büyük/küçük harf, alt çizgi veya
// tire farkına takılabiliyor. Anahtar eşleştirme normalize edilerek yapılır.
func ParseScanResponse(raw string) (ScanResult, error) {
	payload, err := stripCodeFence(raw)
	if err != nil {
		return ScanResult{}, err
	}

	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()

	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return ScanResult{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	normalized := make(map[string]any, len(fields))
	for key, value := range fields {
		normalized[normalizeKey(key)] = value
	}

	if !boolField(normalized, "isreceipt") {
		reason := orDefault(stringField(normalized, "reason"), "Bu görselde okunabilir bir fiş bulunamadı.")
		return ScanResult{}, &NotReceiptError{Reason: reason}
	}

	amount, ok := numberField(normalized, "totalamount")
	if !ok {
		return ScanResult{}, &IncompleteReceiptError{Detail: "total amount is missing"}
	}

	amountText, err := store.NormalizeAmount(amount)
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: err.Error()}
	}
	amount, err = strconv.ParseFloat(amountText, 64)
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: err.Error()}
	}

	storeName, err := store.NormalizeStoreName(stringField(normalized, "storename"))
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: "store name is missing or unusable"}
	}

	currency, err := store.NormalizeCurrency(stringField(normalized, "currency"))
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: err.Error()}
	}

	category, err := store.NormalizeCategory(stringField(normalized, "category"))
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: "category is missing or unusable"}
	}

	receiptDate, err := parseReceiptDate(stringField(normalized, "receiptdate"))
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: err.Error()}
	}

	normalizedDate, err := store.NormalizeReceiptDate(receiptDate)
	if err != nil {
		return ScanResult{}, &IncompleteReceiptError{Detail: err.Error()}
	}

	result := ScanResult{
		StoreName:   storeName,
		TotalAmount: amount,
		Currency:    currency,
		Category:    category,
		ReceiptDate: normalizedDate,
	}

	if tax := strings.TrimSpace(stringField(normalized, "taxnumber")); tax != "" {
		if len(tax) > 64 {
			tax = tax[:64]
		}
		result.TaxNumber = &tax
	}

	return result, nil
}

// receiptDateLayouts, modelin dönebileceği tarih biçimlerini kapsar.
// Belgenin dili ne olursa olsun gün-ay-yıl (Avrupa) varsayılır; belirsiz
// durumlarda yanlış gün seçmektense ay/gün ayrımı riski düşük biçimler
// tercih edilmiştir.
var receiptDateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"02.01.2006 15:04",
	"02.01.2006",
	"02/01/2006",
	"02-01-2006",
	"01/02/2006",
}

// noonUTC, tarih-saat bilgisi olmayan fişlerin gün kaymasını önler: ertesi gün
// yarın ülkeye göre yorumlanmasın diye gün ortasına sabitlenir.
var noonUTC = time.Date(2000, time.January, 1, 12, 0, 0, 0, time.UTC)

func parseReceiptDate(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, errors.New("receipt date is missing")
	}

	for _, layout := range receiptDateLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			if !hasClock(layout) {
				year, month, day := parsed.Date()
				parsed = time.Date(year, month, day, noonUTC.Hour(), 0, 0, 0, time.UTC)
			}
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("receipt date %q could not be parsed", raw)
}

func hasClock(layout string) bool {
	return strings.Contains(layout, ":")
}

// stripCodeFence, modelin JSON'u ```json ... ``` bloğuna sarması hâlinde
// normalize eder.
func stripCodeFence(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%w: empty response", ErrMalformedResponse)
	}

	if strings.HasPrefix(value, "```") {
		value = strings.TrimPrefix(value, "```")
		if idx := strings.IndexByte(value, '\n'); idx >= 0 {
			// İlk satır dil etiketi (```json gibi) olabilir.
			if language := strings.TrimSpace(value[:idx]); language == "" || !strings.Contains(language, "{") {
				value = value[idx+1:]
			}
		}
		if idx := strings.LastIndex(value, "```"); idx >= 0 {
			value = value[:idx]
		}
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: empty response after stripping code fence", ErrMalformedResponse)
	}

	return value, nil
}

func normalizeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func stringField(fields map[string]any, key string) string {
	switch value := fields[key].(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return ""
	}
}

func boolField(fields map[string]any, key string) bool {
	switch value := fields[key].(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	default:
		return false
	}
}

// numberField, model sayıyı hem sayı hem de metin olarak dönebileceği için
// ikisini de kabul eder ("1250,75" gibi yerel ayraçlı metinler de normalize edilir).
func numberField(fields map[string]any, key string) (float64, bool) {
	switch value := fields[key].(type) {
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case float64:
		return value, true
	case string:
		return parseLooseFloat(value)
	default:
		return 0, false
	}
}

func parseLooseFloat(raw string) (float64, bool) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	if cleaned == "" {
		return 0, false
	}

	// Avrupa biçimi: 1.234,56 -> 1234.56
	if strings.Contains(cleaned, ",") {
		cleaned = strings.ReplaceAll(cleaned, ".", "")
		cleaned = strings.ReplaceAll(cleaned, ",", ".")
	} else if strings.Count(cleaned, ".") > 1 {
		// Binlik ayraç olarak nokta kullanılmış olabilir: 1.234.567
		cleaned = strings.ReplaceAll(cleaned, ".", "")
	}

	cleaned = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
			return r
		}
		return -1
	}, cleaned)

	parsed, err := strconv.ParseFloat(cleaned, 64)
	return parsed, err == nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
