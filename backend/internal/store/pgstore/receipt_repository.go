package pgstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selim/fatura-backend/internal/store"
)

const receiptColumns = `id, user_id, store_name, total_amount, currency, category, receipt_date, local_image_path, created_at`

// foreignKeyViolation, PostgreSQL'ın referans bütünlüğü ihlali hata kodu.
const foreignKeyViolation = "23503"

type ReceiptRepository struct {
	pool *pgxpool.Pool
}

func NewReceiptRepository(pool *pgxpool.Pool) *ReceiptRepository {
	return &ReceiptRepository{pool: pool}
}

// Create, doğrulanmış fiş kaydını ekler ve veritabanının ürettiği satırı döner.
// Tutar, domain katmanında normalize edilmiş kanonik metin olarak gönderilir;
// NUMERIC(12,2) kolonu son savunma olarak yine de kendi doğrulamasını yapar.
func (r *ReceiptRepository) Create(ctx context.Context, receipt store.Receipt) (store.Receipt, error) {
	const q = `
		INSERT INTO receipts (
			user_id, store_name, total_amount, currency, category, receipt_date, local_image_path
		)
		VALUES ($1, $2, $3::numeric, $4, $5, $6, $7)
		RETURNING ` + receiptColumns

	amount, err := store.NormalizeAmount(receipt.TotalAmount)
	if err != nil {
		return store.Receipt{}, fmt.Errorf("pgstore: create receipt: %w", err)
	}

	row := r.pool.QueryRow(ctx, q,
		receipt.UserID,
		receipt.StoreName,
		amount,
		receipt.Currency,
		string(receipt.Category),
		receipt.ReceiptDate,
		receipt.LocalImagePath,
	)

	created, err := scanReceipt(row)
	if err != nil {
		// Kullanıcı satırı yoksa 500 değil 401 dönmelidir: oturum geçersiz.
		if isForeignKeyViolation(err) {
			return store.Receipt{}, store.ErrUserNotFound
		}
		return store.Receipt{}, fmt.Errorf("pgstore: create receipt: %w", err)
	}

	return created, nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

// ListByUser, kullanıcının fişlerini filtreli ve sayfalanmış şekilde döner.
// Filtre koşulları dinamik kurulur ancak DEĞERLERİ DAİMA parametre olarak
// gönderilir; hiçbir kullanıcı girdisi SQL metnine gömülmez.
func (r *ReceiptRepository) ListByUser(ctx context.Context, userID uuid.UUID, filter store.ReceiptFilter) ([]store.Receipt, error) {
	normalized, err := filter.Normalize()
	if err != nil {
		return nil, fmt.Errorf("pgstore: list receipts: %w", err)
	}

	var (
		conditions = []string{"user_id = $1"}
		args       = []any{userID}
	)

	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if normalized.Category != "" {
		conditions = append(conditions, "category = "+addArg(string(normalized.Category)))
	}
	if normalized.From != nil {
		conditions = append(conditions, "receipt_date >= "+addArg(*normalized.From))
	}
	if normalized.To != nil {
		// To günü dahil etmek için bir sonraki günün başlangıcı exclusive
		// sınır olarak kullanılır; aksi halde "17.10.2026" filtresi o günü
		// atardı.
		end := normalized.To.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
		conditions = append(conditions, "receipt_date < "+addArg(end))
	}

	limitPlaceholder := addArg(normalized.Limit)
	offsetPlaceholder := addArg(normalized.Offset)

	// created DESC + id ile sabit sıralama, sayfalamada tekrarlı kayıt
	// (created_at aynı olan satırlar) oluşmasını engeller.
	query := `SELECT ` + receiptColumns + `
		FROM receipts
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY created_at DESC, id DESC
		LIMIT ` + limitPlaceholder + ` OFFSET ` + offsetPlaceholder

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list receipts: %w", err)
	}
	defer rows.Close()

	receipts := make([]store.Receipt, 0, normalized.Limit)
	for rows.Next() {
		receipt, err := scanReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("pgstore: scan receipt: %w", err)
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgstore: list receipts: %w", err)
	}

	return receipts, nil
}

func scanReceipt(row pgx.Row) (store.Receipt, error) {
	var receipt store.Receipt

	err := row.Scan(
		&receipt.ID,
		&receipt.UserID,
		&receipt.StoreName,
		&receipt.TotalAmount,
		&receipt.Currency,
		&receipt.Category,
		&receipt.ReceiptDate,
		&receipt.LocalImagePath,
		&receipt.CreatedAt,
	)
	if err != nil {
		return store.Receipt{}, err
	}

	return receipt, nil
}
