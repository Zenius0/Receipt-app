package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// interactionFixture, Gemini Interactions API'nin gerçek yanıt gövdesidir.
func interactionFixture(text string) string {
	body, _ := json.Marshal(map[string]any{
		"created": "2026-10-17T12:00:00Z",
		"id":      "v1_test",
		"model":   DefaultGeminiModel,
		"object":  "interaction",
		"status":  "completed",
		"steps": []map[string]any{
			{
				"type": "model_output",
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	})
	return string(body)
}

func newTestGemini(t *testing.T, handler http.HandlerFunc) *GeminiScanner {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	scanner, err := NewGeminiScanner("test-key", DefaultGeminiModel, 5*time.Second)
	if err != nil {
		t.Fatalf("NewGeminiScanner: %v", err)
	}
	scanner.baseURL = server.URL

	return scanner
}

func TestGeminiScanner_Scan(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	var captured map[string]any

	scanner := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1beta/interactions" {
			t.Errorf("path = %q, want /v1beta/interactions", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("x-goog-api-key = %q, want test-key", got)
		}

		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &captured); err != nil {
			t.Errorf("request body is not json: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(interactionFixture(
			`{"isReceipt":true,"storeName":"Beko","totalAmount":899.90,"currency":"TRY","category":"repair","receiptDate":"` + date + `"}`,
		)))
	})

	image := Image{Data: tinyPNG, MimeType: "image/png"}

	result, err := scanner.Scan(context.Background(), image)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if result.StoreName != "Beko" || result.TotalAmount != 899.90 {
		t.Errorf("unexpected result: %+v", result)
	}

	// İstek gövdesi beklenen sözleşmeyi taşımalı: model, görsel ve şema.
	if captured["model"] != DefaultGeminiModel {
		t.Errorf("model = %v, want %s", captured["model"], DefaultGeminiModel)
	}

	input, ok := captured["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("input = %v, want 2 parts", captured["input"])
	}
	imagePart, ok := input[1].(map[string]any)
	if !ok {
		t.Fatalf("second input part is not an object: %v", input[1])
	}
	if imagePart["type"] != "image" || imagePart["mime_type"] != "image/png" {
		t.Errorf("image part = %v, want type=image mime_type=image/png", imagePart)
	}
	if imagePart["data"] != base64.StdEncoding.EncodeToString(tinyPNG) {
		t.Error("image data was not forwarded as base64")
	}

	format, ok := captured["response_format"].(map[string]any)
	if !ok || format["mime_type"] != "application/json" {
		t.Errorf("response_format = %v, want application/json", captured["response_format"])
	}

	// Gemini 3.6+ modeller temperature'ı yok sayıp uyarı döner; gönderilmemeli.
	config, ok := captured["generation_config"].(map[string]any)
	if !ok {
		t.Fatalf("generation_config missing: %v", captured["generation_config"])
	}
	if _, has := config["temperature"]; has {
		t.Error("generation_config must not contain temperature for Gemini 3.6+ models")
	}
}

func TestGeminiScanner_NotAReceipt(t *testing.T) {
	scanner := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(interactionFixture(`{"isReceipt":false,"reason":"Bu bir tabak yemek fotoğrafı."}`)))
	})

	_, err := scanner.Scan(context.Background(), Image{Data: tinyPNG, MimeType: "image/png"})
	if !errors.Is(err, ErrNotReceipt) {
		t.Fatalf("error = %v, want ErrNotReceipt", err)
	}
	if !strings.Contains(err.Error(), "tabak yemek") {
		t.Errorf("error = %v, want model açıklamasını taşımak", err)
	}
}

func TestGeminiScanner_UpstreamErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUpstreamAuth},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"server error", http.StatusInternalServerError, ErrUpstream},
		{"bad request", http.StatusBadRequest, ErrUpstream},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int
			scanner := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
				attempts++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":{"message":"boom","status":"FAILED_PRECONDITION"}}`))
			})

			_, err := scanner.Scan(context.Background(), Image{Data: tinyPNG, MimeType: "image/png"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}

			// 429 ve 5xx yeniden denenir, 4xx'te tek deneme yapılır.
			wantAttempts := 1
			if tc.want == ErrRateLimited || tc.want == ErrUpstream && tc.status >= 500 {
				wantAttempts = 3
			}
			if attempts != wantAttempts {
				t.Errorf("attempts = %d, want %d", attempts, wantAttempts)
			}
		})
	}
}

func TestGeminiScanner_TruncatedInteraction(t *testing.T) {
	scanner := newTestGemini(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"incomplete","steps":[]}`))
	})

	_, err := scanner.Scan(context.Background(), Image{Data: tinyPNG, MimeType: "image/png"})
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

func TestNewGeminiScanner_RequiresAPIKey(t *testing.T) {
	if _, err := NewGeminiScanner("   ", DefaultGeminiModel, time.Second); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("error = %v, want ErrNotConfigured", err)
	}
}

func TestOpenAIScanner_Scan(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"isReceipt\":true,\"storeName\":\"Apple Store\",\"totalAmount\":4999.00,\"currency\":\"TRY\",\"category\":\"software\",\"receiptDate\":\"` + date + `\"}"}}]}`))
	}))
	t.Cleanup(server.Close)

	scanner, err := NewOpenAIScanner("test-key", DefaultOpenAIModel, 5*time.Second)
	if err != nil {
		t.Fatalf("NewOpenAIScanner: %v", err)
	}
	scanner.baseURL = server.URL

	result, err := scanner.Scan(context.Background(), Image{Data: tinyPNG, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.StoreName != "Apple Store" || result.TotalAmount != 4999.00 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestOpenAIResponseSchema_StrictRequiresEveryField(t *testing.T) {
	schema := openAIResponseSchema()

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("schema has no required list")
	}
	if len(required) != 8 {
		t.Errorf("required has %d entries, want 8 (strict mod tüm alanları ister)", len(required))
	}
	if schema["additionalProperties"] != false {
		t.Error("additionalProperties must be false in strict mode")
	}
}
