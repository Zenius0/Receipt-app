package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrInvalidCategory = errors.New("store: unknown receipt category")
	ErrInvalidCurrency = errors.New("store: currency must be a 3 letter ISO 4217 code")
	ErrInvalidAmount   = errors.New("store: total amount must be positive")
	ErrInvalidDate     = errors.New("store: receipt date is out of range")
	ErrInvalidStore    = errors.New("store: store name is required")
	// ErrUserNotFound, fişin sahibi olan kullanıcının artık var olmadığını
	// belirtir (hesap silinmiş veya yanlışlıkla düşmüş olabilir).
	ErrUserNotFound = errors.New("store: owner user does not exist")
)

// Category, fiş harcamasının normalize edilmiş sınıfıdır. Değerler veritabanındaki
// receipts_category_chk kısıtıyla birebir aynı olmalıdır; AllCategories bu
// listeyi tek doğruluk kaynağı olarak sunar.
type Category string

const (
	CategoryFuel          Category = "fuel"
	CategoryFood          Category = "food"
	CategoryGroceries     Category = "groceries"
	CategoryRepair        Category = "repair"
	CategorySoftware      Category = "software"
	CategoryOffice        Category = "office"
	CategoryTravel        Category = "travel"
	CategoryHealth        Category = "health"
	CategoryEntertainment Category = "entertainment"
	CategoryClothing      Category = "clothing"
	CategoryUtilities     Category = "utilities"
	CategoryOther         Category = "other"
)

var categoryOrder = []Category{
	CategoryFuel,
	CategoryFood,
	CategoryGroceries,
	CategoryRepair,
	CategorySoftware,
	CategoryOffice,
	CategoryTravel,
	CategoryHealth,
	CategoryEntertainment,
	CategoryClothing,
	CategoryUtilities,
	CategoryOther,
}

// AllCategories, DB kısıtıyla ve AI prompt şemasıyla paylaşılan kategori listesini döner.
func AllCategories() []Category {
	out := make([]Category, len(categoryOrder))
	copy(out, categoryOrder)
	return out
}

// categoryAliases, yapay zekanın dönebileceği serbest metinleri kanonik değerlere eşler.
// AI JSON şemasına enum olarak da verildiği için bu tablo yalnızca savunma amaçlıdır:
// sürücü sürümü farklılıkları, Türkçe/İngilizce karışımı veya prompt kaçırması
// halinde satırı düşürmek yerine yanlış kategoriye sessizce kaydetmektense
// hatayı görünür kılmayı tercih ederiz.
var categoryAliases = map[string]Category{
	"yakıt": CategoryFuel, "yakit": CategoryFuel, "akaryakit": CategoryFuel, "akaryakıt": CategoryFuel,
	"fuel": CategoryFuel, "gasoline": CategoryFuel, "petrol": CategoryFuel, "benzin": CategoryFuel,
	"yemek": CategoryFood, "restaurant": CategoryFood, "restoran": CategoryFood, "lokanta": CategoryFood,
	"food": CategoryFood, "dining": CategoryFood, "cafe": CategoryFood, "kahve": CategoryFood,
	"bakkal": CategoryGroceries, "market": CategoryGroceries, "alışveriş": CategoryGroceries, "alisveris": CategoryGroceries,
	"groceries": CategoryGroceries, "grocery": CategoryGroceries, "supermarket": CategoryGroceries,
	"tamir": CategoryRepair, "servis": CategoryRepair, "repair": CategoryRepair, "maintenance": CategoryRepair,
	"bakim": CategoryRepair, "bakım": CategoryRepair,
	"yazılım": CategorySoftware, "yazilim": CategorySoftware, "software": CategorySoftware,
	"saas": CategorySoftware, "subscription": CategorySoftware, "abonelik": CategorySoftware,
	"lisans": CategorySoftware, "license": CategorySoftware, "hosting": CategorySoftware,
	"kırtasiye": CategoryOffice, "kirtasiye": CategoryOffice, "office": CategoryOffice, "office supply": CategoryOffice,
	"seyahat": CategoryTravel, "travel": CategoryTravel, "otel": CategoryTravel, "hotel": CategoryTravel,
	"uçak": CategoryTravel, "ucak": CategoryTravel, "flight": CategoryTravel, "taksi": CategoryTravel, "taxi": CategoryTravel,
	"sağlık": CategoryHealth, "saglik": CategoryHealth, "health": CategoryHealth,
	"eczane": CategoryHealth, "pharmacy": CategoryHealth, "doktor": CategoryHealth,
	"eğlence": CategoryEntertainment, "eglence": CategoryEntertainment, "entertainment": CategoryEntertainment,
	"sinema": CategoryEntertainment, "cinema": CategoryEntertainment, "konser": CategoryEntertainment,
	"giyim": CategoryClothing, "clothing": CategoryClothing, "apparel": CategoryClothing,
	"kıyafet": CategoryClothing, "kiyafet": CategoryClothing, "ayakkabı": CategoryClothing, "ayakkabi": CategoryClothing,
	"fatura": CategoryUtilities, "utilities": CategoryUtilities, "utility": CategoryUtilities,
	"elektrik": CategoryUtilities, "su": CategoryUtilities, "doğalgaz": CategoryUtilities, "dogalgaz": CategoryUtilities,
	"internet": CategoryUtilities, "telefon": CategoryUtilities,
	"diğer": CategoryOther, "diger": CategoryOther, "other": CategoryOther, "others": CategoryOther,
	"misc": CategoryOther, "unknown": CategoryOther,
}

// NormalizeCategory, serbest metni kanonik bir kategoriye indirger.
func NormalizeCategory(raw string) (Category, error) {
	key := normalizeKey(raw)
	if key == "" {
		return "", fmt.Errorf("%w: empty value", ErrInvalidCategory)
	}

	if c, ok := categoryAliases[key]; ok {
		return c, nil
	}
	// AI çıktısı birebir kanonik değer ise (enum ihlali veya kuru çalıştırma).
	for _, c := range categoryOrder {
		if string(c) == key {
			return c, nil
		}
	}

	return "", fmt.Errorf("%w: %q", ErrInvalidCategory, raw)
}

// currencyAliases, yaygın sembol ve ifadeleri ISO 4217 koduna eşler.
var currencyAliases = map[string]string{
	"₺": "TRY", "try": "TRY", "tl": "TRY", "türk lirası": "TRY", "turk lirasi": "TRY",
	"$": "USD", "usd": "USD", "us$": "USD", "dolar": "USD", "dollar": "USD", "dollars": "USD",
	"€": "EUR", "eur": "EUR", "euro": "EUR", "euros": "EUR",
	"£": "GBP", "gbp": "GBP", "pound": "GBP", "sterlin": "GBP",
	"¥": "JPY", "jpy": "JPY", "yen": "JPY",
	"₺₺": "TRY",
}

// NormalizeCurrency, sembol/kısaltmayı büyük harfli 3 harflik ISO 4217 koduna çevirir.
func NormalizeCurrency(raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", fmt.Errorf("%w: empty value", ErrInvalidCurrency)
	}

	if code, ok := currencyAliases[key]; ok {
		return code, nil
	}

	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != 3 {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, raw)
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, raw)
		}
	}

	return code, nil
}

// maxAmount, NUMERIC(12,2) kolonunun taşabileceği en büyük değerdir.
const maxAmount = 9_999_999_999.99

// NormalizeAmount, tutarı iki ondalıklı kanonik metne yuvarlar.
// float64 -> metin dönüşümü bilinçlidir: PostgreSQL tarafındaki NUMERIC(12,2)
// doğrulaması her durumda son savunmadır.
func NormalizeAmount(amount float64) (string, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return "", fmt.Errorf("%w: not a finite number", ErrInvalidAmount)
	}
	if amount < 0 {
		return "", fmt.Errorf("%w: %v is negative", ErrInvalidAmount, amount)
	}
	if amount > maxAmount {
		return "", fmt.Errorf("%w: %v exceeds maximum", ErrInvalidAmount, amount)
	}

	return strconv.FormatFloat(math.Round(amount*100)/100, 'f', 2, 64), nil
}

const (
	maxStoreNameRunes = 200
	// Fiş tarihi bu kadar eski olamaz; çok eski kayıtlar muhtemelen OCR hatasıdır.
	oldestReceiptDate = 10 * 365 * 24 * time.Hour
	// Gelecek toleransı cihaz saat farkları ve "bugünün fişi" yorumu için.
	futureReceiptSkew = 24 * time.Hour
)

// NormalizeStoreName, işletme adını boşluk ve uzunluk açısından temizler.
func NormalizeStoreName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", ErrInvalidStore
	}
	if len([]rune(name)) > maxStoreNameRunes {
		return "", fmt.Errorf("%w: longer than %d characters", ErrInvalidStore, maxStoreNameRunes)
	}

	return name, nil
}

// NormalizeReceiptDate, fiş tarihini makul bir aralığa çeker.
func NormalizeReceiptDate(t time.Time) (time.Time, error) {
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("%w: empty date", ErrInvalidDate)
	}

	utc := t.UTC()
	now := time.Now().UTC()

	if utc.After(now.Add(futureReceiptSkew)) {
		return time.Time{}, fmt.Errorf("%w: %s is in the future", ErrInvalidDate, utc.Format(time.RFC3339))
	}
	if utc.Before(now.Add(-oldestReceiptDate)) {
		return time.Time{}, fmt.Errorf("%w: %s is too old", ErrInvalidDate, utc.Format(time.RFC3339))
	}

	return utc, nil
}

// Receipt, veritabanındaki receipts satırının Go temsilidir.
type Receipt struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"userId"`
	StoreName      string    `json:"storeName"`
	TotalAmount    float64   `json:"totalAmount"`
	Currency       string    `json:"currency"`
	Category       Category  `json:"category"`
	ReceiptDate    time.Time `json:"receiptDate"`
	LocalImagePath *string   `json:"localImagePath"`
	CreatedAt      time.Time `json:"createdAt"`
}

// NewReceipt, doğrulanmış bir kayıt oluşturur. Alanlar veritabanı kısıtlarıyla
// aynı kurallara göre normalize edilir; iki katmanlı savunma (domain + DB)
// sayesinde bozuk veri yazılmaz.
func NewReceipt(userID uuid.UUID, storeName string, totalAmount float64, currency string, category string, receiptDate time.Time, localImagePath *string) (Receipt, error) {
	if userID == uuid.Nil {
		return Receipt{}, errors.New("store: user id is required")
	}

	name, err := NormalizeStoreName(storeName)
	if err != nil {
		return Receipt{}, err
	}
	amount, err := NormalizeAmount(totalAmount)
	if err != nil {
		return Receipt{}, err
	}
	parsedAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return Receipt{}, fmt.Errorf("%w: %w", ErrInvalidAmount, err)
	}
	code, err := NormalizeCurrency(currency)
	if err != nil {
		return Receipt{}, err
	}
	cat, err := NormalizeCategory(category)
	if err != nil {
		return Receipt{}, err
	}
	date, err := NormalizeReceiptDate(receiptDate)
	if err != nil {
		return Receipt{}, err
	}

	path := localImagePath
	if path != nil {
		trimmed := strings.TrimSpace(*path)
		if trimmed == "" {
			path = nil
		} else if len(trimmed) > 1024 {
			return Receipt{}, errors.New("store: local image path is too long")
		} else {
			path = &trimmed
		}
	}

	return Receipt{
		ID:             uuid.New(),
		UserID:         userID,
		StoreName:      name,
		TotalAmount:    parsedAmount,
		Currency:       code,
		Category:       cat,
		ReceiptDate:    date,
		LocalImagePath: path,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

type ReceiptRepository interface {
	Create(ctx context.Context, receipt Receipt) (Receipt, error)
}

// normalizeKey, kategori/sembol karşılaştırmaları için küçük harf, aksan ve
// boşluk normalize eder.
func normalizeKey(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))

	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '/':
			b.WriteRune(' ')
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}
