package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient, sağlayıcı REST çağrıları için ortak, yeniden deneme yeteneğine
// sahip ince bir istemcidir.
type httpClient struct {
	client      *http.Client
	maxAttempts int
}

func newHTTPClient(timeout time.Duration) *httpClient {
	return &httpClient{
		client:      &http.Client{Timeout: timeout},
		maxAttempts: 3,
	}
}

// backoff, üstel geri çekilme ile deterministik (jitter'sız) bekleme.
// Mobil kullanıcı önünde beklemek yerine hızlıca başarısız olmak daha iyi
// olduğu için toplam bekleme bilinçli olarak kısa tutulur.
func backoff(attempt int) time.Duration {
	return time.Duration(attempt*attempt) * 400 * time.Millisecond
}

// retryableStatus, geçici sağlayıcı hataları için yeniden denemeye değer HTTP
// kodlarıdır. 429 kapsanır; 4xx'in kalanı istemci hatasıdır (anahtar, şema,
// kota kredisi) ve tekrar denemek yalnızca geciktirir.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusRequestTimeout ||
		code >= http.StatusInternalServerError
}

// apiError, sağlayıcının gövde döndürdüğü ortak hata biçimi.
type apiError struct {
	Message string `json:"message"`
	Status  string `json:"status"`
	Type    string `json:"type"`
}

func (e *apiError) String() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Status
}

func decodeAPIError(body []byte) string {
	var wrapper struct {
		Error *apiError `json:"error"`
		Msg   string    `json:"message"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil {
		if wrapper.Error != nil {
			return wrapper.Error.String()
		}
		if wrapper.Msg != "" {
			return wrapper.Msg
		}
	}
	return strings.TrimSpace(string(body))
}

// postJSON, isteği gönderir ve başarılı yanıtı out'a çözer. Sağlayıcı hataları
// sentinel hatalara (ErrRateLimited, ErrUpstreamAuth, ErrUpstream) eşlenir ki
// handler katmanı kullanıcıya anlamlı bir mesap dönebilsin.
func (c *httpClient) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ai: encode request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if attempt > 1 {
			timer := time.NewTimer(backoff(attempt - 1))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		status, respBody, err := c.do(ctx, endpoint, headers, body)
		if err != nil {
			// Bağlantı hatası: bağlam iptal edilmediyse tekrar dene.
			if ctx.Err() != nil {
				return err
			}
			lastErr = err
			continue
		}

		if status >= 200 && status < 300 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("%w: %v", ErrMalformedResponse, err)
			}
			return nil
		}

		providerMsg := decodeAPIError(respBody)
		if retryableStatus(status) && attempt < c.maxAttempts {
			lastErr = fmt.Errorf("ai: provider returned %d: %s", status, providerMsg)
			continue
		}

		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return fmt.Errorf("%w: provider returned %d: %s", ErrUpstreamAuth, status, providerMsg)
		case status == http.StatusTooManyRequests:
			return fmt.Errorf("%w: provider returned %d: %s", ErrRateLimited, status, providerMsg)
		default:
			return fmt.Errorf("%w: provider returned %d: %s", ErrUpstream, status, providerMsg)
		}
	}

	if lastErr == nil {
		lastErr = errors.New("ai: provider request failed")
	}
	return lastErr
}

func (c *httpClient) do(ctx context.Context, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("ai: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ai: call provider: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Sağlayıcı hata gövdeleri de JSON olabilir; 1 MB ile sınırla.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("ai: read response: %w", err)
	}

	return resp.StatusCode, respBody, nil
}
