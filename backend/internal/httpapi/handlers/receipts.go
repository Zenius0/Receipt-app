package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/selim/fatura-backend/internal/ai"
	"github.com/selim/fatura-backend/internal/httpx"
	"github.com/selim/fatura-backend/internal/middleware"
	"github.com/selim/fatura-backend/internal/receipts"
	"github.com/selim/fatura-backend/internal/store"
)

// maxScanBodyBytes, scan isteğinin ham gövde sınırıdır.
// 8 MB'lık bir görsel base64 olarak yaklaşık 10,7 MB'a dönüşür; multipart
// sarmalayıcı için biraz pay bırakıldı.
const maxScanBodyBytes = 12 << 20

// multipartImageFields, mobil istemcilerin gönderdiği alan adlarıdır.
var multipartImageFields = []string{"image", "file", "receipt", "photo"}

type ReceiptHandler struct {
	service       *receipts.Service
	logger        *slog.Logger
	maxImageBytes int
}

func NewReceiptHandler(service *receipts.Service, logger *slog.Logger, maxImageBytes int) *ReceiptHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if maxImageBytes <= 0 {
		maxImageBytes = ai.MaxImageBytes
	}

	return &ReceiptHandler{service: service, logger: logger, maxImageBytes: maxImageBytes}
}

// scanResponse, istemcinin kullanıcıya onay ettireceği öneri gövdesidir.
type scanResponse struct {
	StoreName   string    `json:"storeName"`
	TotalAmount float64   `json:"totalAmount"`
	Currency    string    `json:"currency"`
	Category    string    `json:"category"`
	ReceiptDate time.Time `json:"receiptDate"`
	TaxNumber   *string   `json:"taxNumber,omitempty"`
}

// scanJSONRequest, base64 gövde gönderen istemciler için kullanılır.
type scanJSONRequest struct {
	ImageBase64 string `json:"imageBase64"`
	MimeType    string `json:"mimeType"`
}

// Scan, mobil cihazdan gelen fiş görselini alır, yapay zekâya gönderir ve
// ayrıştırılmış veriyi kullanıcı onayı için döner. Görsel sunucuda saklanmaz.
func (h *ReceiptHandler) Scan(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
		return
	}

	image, err := h.readImage(w, r)
	if err != nil {
		h.writeScanError(w, r, err)
		return
	}

	// Sağlayıcının kendi zaman aşımından da kısa tutulur; Render'ın idle
	// timeout'ına takılmadan kullanıcıya anlamlı hata dönebilmek için.
	ctx, cancel := context.WithTimeout(r.Context(), aiRequestTimeout)
	defer cancel()

	result, err := h.service.Scan(ctx, image)
	if err != nil {
		h.writeScanError(w, r, err)
		return
	}

	w.Header().Set("X-AI-Model", h.service.ScannerName())

	h.logger.InfoContext(r.Context(), "receipt scan completed",
		slog.String("user_id", claims.UserID.String()),
		slog.String("request_id", middleware.RequestIDFromContext(r.Context())),
	)

	httpx.WriteJSON(w, http.StatusOK, scanResponse{
		StoreName:   result.StoreName,
		TotalAmount: result.TotalAmount,
		Currency:    result.Currency,
		Category:    string(result.Category),
		ReceiptDate: result.ReceiptDate,
		TaxNumber:   result.TaxNumber,
	})
}

// readImage, Content-Type'a göre multipart ya da base64 gövdesini okur ve
// doğrulanmış bir ai.Image döndürür.
func (h *ReceiptHandler) readImage(w http.ResponseWriter, r *http.Request) (ai.Image, error) {
	mediaType := contentTypeOf(r)

	switch {
	case strings.HasPrefix(mediaType, "multipart/form-data"):
		return h.readMultipartImage(w, r)
	case mediaType == "application/json" || mediaType == "":
		return h.readBase64Image(w, r)
	default:
		return ai.Image{}, fmt.Errorf("%w: content type %q is not supported, use multipart/form-data or application/json", errUnsupportedPayload, mediaType)
	}
}

var errUnsupportedPayload = errors.New("handlers: unsupported payload")

func (h *ReceiptHandler) readMultipartImage(w http.ResponseWriter, r *http.Request) (ai.Image, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxScanBodyBytes)
	defer func() { _ = r.Body.Close() }()

	if err := r.ParseMultipartForm(maxScanBodyBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return ai.Image{}, fmt.Errorf("%w: image exceeds the upload limit", ai.ErrImageTooLarge)
		}
		return ai.Image{}, fmt.Errorf("%w: multipart body could not be parsed", ai.ErrMalformedImage)
	}

	for _, field := range multipartImageFields {
		file, header, err := r.FormFile(field)
		if err != nil {
			continue
		}
		defer func() { _ = file.Close() }()

		if header.Size > int64(h.maxImageBytes) {
			return ai.Image{}, fmt.Errorf("%w: %d bytes exceeds limit of %d", ai.ErrImageTooLarge, header.Size, h.maxImageBytes)
		}

		data, err := io.ReadAll(io.LimitReader(file, int64(h.maxImageBytes)+1))
		if err != nil {
			return ai.Image{}, fmt.Errorf("%w: could not read uploaded file", ai.ErrMalformedImage)
		}

		return ai.ValidateImage(data, h.maxImageBytes)
	}

	return ai.Image{}, fmt.Errorf("%w: multipart field %q is missing", ai.ErrMalformedImage, multipartImageFields[0])
}

func (h *ReceiptHandler) readBase64Image(w http.ResponseWriter, r *http.Request) (ai.Image, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxScanBodyBytes)
	defer func() { _ = r.Body.Close() }()

	var req scanJSONRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return ai.Image{}, fmt.Errorf("%w: payload exceeds the upload limit", ai.ErrImageTooLarge)
		}
		if errors.Is(err, io.EOF) {
			return ai.Image{}, fmt.Errorf("%w: request body is required", ai.ErrMalformedImage)
		}
		return ai.Image{}, fmt.Errorf("%w: body must be valid json", ai.ErrMalformedImage)
	}

	return ai.DecodeBase64Image(req.ImageBase64, h.maxImageBytes)
}

// confirmRequest, kullanıcının onayladığı veridir. Tutar sayı, tarih ise açık
// RFC3339 metni olarak beklenir; belirsiz tarih biçimleri burada reddedilir.
type confirmRequest struct {
	StoreName      string  `json:"storeName"`
	TotalAmount    float64 `json:"totalAmount"`
	Currency       string  `json:"currency"`
	Category       string  `json:"category"`
	ReceiptDate    string  `json:"receiptDate"`
	LocalImagePath *string `json:"localImagePath"`
}

// Confirm, kullanıcının onayladığı fiş verisini ve cihaz içi görsel yolunu
// receipts tablosuna kaydeder.
func (h *ReceiptHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
		return
	}

	var req confirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	receiptDate, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ReceiptDate))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "receiptDate must be an RFC3339 timestamp")
		return
	}

	receipt, err := h.service.Confirm(r.Context(), claims.UserID, receipts.ConfirmInput{
		StoreName:      req.StoreName,
		TotalAmount:    req.TotalAmount,
		Currency:       req.Currency,
		Category:       req.Category,
		ReceiptDate:    receiptDate,
		LocalImagePath: req.LocalImagePath,
	})
	if err != nil {
		h.writeConfirmError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, receipt)
}

// aiRequestTimeout, yapay zekâ çağrısının istemciye bağlı olmayan üst sınırıdır.
// config.ScanTimeout ile aynı değer kullanılır; burada handler paketinin
// config'e bağımlı olmaması için sabit tutulur.
const aiRequestTimeout = 45 * time.Second

func (h *ReceiptHandler) writeScanError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ai.ErrImageTooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image_too_large", "görsel boyutu sınırı aşıyor")
	case errors.Is(err, ai.ErrUnsupportedImage):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported_image", "desteklenmeyen görsel formatı (JPEG, PNG, WEBP veya HEIC)")
	case errors.Is(err, ai.ErrMalformedImage), errors.Is(err, errUnsupportedPayload):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_image", err.Error())
	case errors.Is(err, ai.ErrNotReceipt):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "not_a_receipt", detailOf(err, "Bu görselde okunabilir bir fiş bulunamadı."))
	case errors.Is(err, ai.ErrIncompleteReceipt):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "incomplete_receipt", "Fiş geçerli ama bazı alanlar okunamadı: "+detailOf(err, "bilinmeyen alan"))
	case errors.Is(err, ai.ErrRateLimited):
		httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "yapay zekâ servisi şu anda yoğun, lütfen tekrar deneyin")
	case errors.Is(err, ai.ErrUpstreamAuth), errors.Is(err, ai.ErrNotConfigured):
		h.logInternal(r, "ai provider misconfigured", err)
		httpx.WriteError(w, http.StatusInternalServerError, "ai_not_configured", "yapay zekâ servisi yapılandırılmamış")
	case errors.Is(err, ai.ErrUpstream), errors.Is(err, ai.ErrMalformedResponse):
		httpx.WriteError(w, http.StatusBadGateway, "ai_unavailable", "yapay zekâ servisi şu yanıtı veremedi")
	case errors.Is(err, context.DeadlineExceeded):
		httpx.WriteError(w, http.StatusGatewayTimeout, "ai_timeout", "yapay zekâ servisi zaman aşımına uğradı")
	default:
		h.logInternal(r, "receipt scan failed", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected error")
	}
}

func (h *ReceiptHandler) writeConfirmError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *receipts.InvalidInputError
	if errors.As(err, &invalid) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_receipt", invalid.Detail)
		return
	}

	// Token geçerli ama kullanıcı satırı yok: oturum artık kullanılamaz.
	if errors.Is(err, store.ErrUserNotFound) {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "session user no longer exists")
		return
	}

	h.logInternal(r, "receipt confirm failed", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected error")
}

func (h *ReceiptHandler) logInternal(r *http.Request, message string, err error) {
	h.logger.ErrorContext(r.Context(), message,
		slog.String("error", err.Error()),
		slog.String("request_id", middleware.RequestIDFromContext(r.Context())),
	)
}

// detailOf, yapay zekânın döndürdüğü insan okunabilir açıklamayı taşır.
// Taşıyan hata tipi yoksa fallback kullanılır.
func detailOf(err error, fallback string) string {
	var notReceipt *ai.NotReceiptError
	if errors.As(err, &notReceipt) && notReceipt.Reason != "" {
		return notReceipt.Reason
	}

	var incomplete *ai.IncompleteReceiptError
	if errors.As(err, &incomplete) && incomplete.Detail != "" {
		return incomplete.Detail
	}

	return fallback
}

func contentTypeOf(r *http.Request) string {
	mediaType := r.Header.Get("Content-Type")
	if idx := strings.IndexByte(mediaType, ';'); idx >= 0 {
		mediaType = mediaType[:idx]
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}
