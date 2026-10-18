// Package receipts, fiş çıkarma (scan) ve kaydetme (confirm) iş akışını
// yürüten domain servisidir. Handler katmanı yalnızca HTTP ayrıntılarıyla
// uğraşır; iş kuralları ve sağlayıcı seçimi burada toplanır.
package receipts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/store"
)

// ErrInvalidInput, istemciden gelen onay verisinin kaydedilemeyeceğini belirtir.
// Handler bu hatayı 400/422 ile kullanıcıya döner.
var ErrInvalidInput = errors.New("receipts: invalid input")

// InvalidInputError, reddedilen alanın gerekçesini taşır.
type InvalidInputError struct {
	Detail string
}

func (e *InvalidInputError) Error() string { return "receipts: invalid input: " + e.Detail }

func (e *InvalidInputError) Unwrap() error { return ErrInvalidInput }

type Service struct {
	scanner ai.Scanner
	repo    store.ReceiptRepository
	logger  *slog.Logger
}

func NewService(repo store.ReceiptRepository, scanner ai.Scanner, logger *slog.Logger) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("receipts: receipt repository is required")
	}
	if scanner == nil {
		return nil, fmt.Errorf("receipts: scanner is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{scanner: scanner, repo: repo, logger: logger}, nil
}

// ScannerName, aktif yapay zekâ sağlayıcısının adını döner (log/header için).
func (s *Service) ScannerName() string { return s.scanner.Name() }

// Scan, doğrulanmış fiş görselini yapay zekâya gönderir ve istemciye onay için
// dönecek veriyi üretir. Bu aşamada hiçbir veri kalıcılaştırılmaz: kullanıcı
// değerleri görmediği sürece veritabanına hiçbir şey yazılmaz.
func (s *Service) Scan(ctx context.Context, image ai.Image) (ai.ScanResult, error) {
	started := time.Now()

	result, err := s.scanner.Scan(ctx, image)
	if err != nil {
		return ai.ScanResult{}, err
	}

	s.logger.InfoContext(ctx, "receipt scanned",
		slog.String("provider", s.scanner.Name()),
		slog.String("store_name", result.StoreName),
		slog.String("currency", result.Currency),
		slog.String("category", string(result.Category)),
		slog.Int("image_bytes", len(image.Data)),
		slog.Duration("latency", time.Since(started)),
	)

	return result, nil
}

// ListInput, fiş listeleme isteğinin filtreleridir.
type ListInput struct {
	Category string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

// ListResult, sayfalanmış fiş listesini taşır.
// nextOffset, bir sonraki sayfanın başlangıcıdır; liste dolduysa nil'dir
// ve istemci "daha fazla yok" diyerek isteği durdurur.
type ListResult struct {
	Items      []store.Receipt `json:"items"`
	NextOffset *int            `json:"nextOffset,omitempty"`
	Total      int             `json:"total"`
}

// List, kullanıcının fişlerini döner. Kullanıcı kimliği JWT'den gelir ve
// sorgu her zaman o kullanıcıyla sınırlıdır.
func (s *Service) List(ctx context.Context, userID uuid.UUID, in ListInput) (ListResult, error) {
	filter := store.ReceiptFilter{
		Category: store.Category(in.Category),
		From:     in.From,
		To:       in.To,
		Limit:    in.Limit,
		Offset:   in.Offset,
	}

	normalized, err := filter.Normalize()
	if err != nil {
		return ListResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	items, err := s.repo.ListByUser(ctx, userID, normalized)
	if err != nil {
		return ListResult{}, fmt.Errorf("receipts: list: %w", err)
	}

	result := ListResult{Items: items, Total: len(items)}

	// Tam sayfa döndüyse muhtemelen devamı vardır; istemci bir sonraki istekte
	// bu ofseti kullanır.
	if len(items) == normalized.Limit {
		next := normalized.Offset + normalized.Limit
		result.NextOffset = &next
	}

	s.logger.InfoContext(ctx, "receipts listed",
		slog.String("user_id", userID.String()),
		slog.Int("returned", len(items)),
		slog.Int("limit", normalized.Limit),
		slog.Int("offset", normalized.Offset),
		slog.Bool("has_category_filter", normalized.Category != ""),
	)

	return result, nil
}

// ConfirmInput, kullanıcının mobilde onayladığı nihai fiş verisidir.
// Görsel bu aşamada sunucuya tekrar gönderilmez; yalnızca cihaz içi yol gelir.
type ConfirmInput struct {
	StoreName      string
	TotalAmount    float64
	Currency       string
	Category       string
	ReceiptDate    time.Time
	LocalImagePath *string
}

// Confirm, onaylanan fişi doğrulayıp receipts tablosuna yazar.
// Doğrulama store.NewReceipt içinde yapılır; aynı kurallar veritabanı CHECK
// kısıtlarında da tekrar uygulanır.
func (s *Service) Confirm(ctx context.Context, userID uuid.UUID, in ConfirmInput) (store.Receipt, error) {
	receipt, err := store.NewReceipt(
		userID,
		in.StoreName,
		in.TotalAmount,
		in.Currency,
		in.Category,
		in.ReceiptDate,
		in.LocalImagePath,
	)
	if err != nil {
		return store.Receipt{}, &InvalidInputError{Detail: err.Error()}
	}

	created, err := s.repo.Create(ctx, receipt)
	if err != nil {
		return store.Receipt{}, fmt.Errorf("receipts: confirm: %w", err)
	}

	s.logger.InfoContext(ctx, "receipt saved",
		slog.String("receipt_id", created.ID.String()),
		slog.String("user_id", userID.String()),
		slog.String("category", string(created.Category)),
		slog.String("currency", created.Currency),
		slog.Bool("has_local_image", created.LocalImagePath != nil),
	)

	return created, nil
}
