package pgstore

import (
	"context"
	"errors"
	"fmt"

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
