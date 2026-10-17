package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// DefaultGeminiModel, fiş çıkarımı için güncel Flash modeli.
// Fiyat/performans dengesi ve yapılandırılmış çıktı desteği nedeniyle
// varsayılan olarak Flash katmanı seçilir; GEMINI_MODEL ile değiştirilebilir.
const DefaultGeminiModel = "gemini-3.8-flash"

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

// GeminiScanner, Google Gemini Interactions API'sini konuşur.
//
// API notu: Gemini, eski :generateContent uç noktası yerine
// /v1beta/interactions uç noktasını kullanır. Yanıt gövdesinde model çıktısı
// steps[].content[] içinde, type "text" olan bloklarda döner.
//
// Üretim notu: Gemini 3.6+ modeller temperature/top_p/top_k değerlerini yok
// sayar ve uyarı başlığı döner; bu yüzden istekte hiç örnekleme parametresi
// gönderilmez. Determinizm yanıt şemasından (response_format) gelir.
type GeminiScanner struct {
	apiKey  string
	model   string
	baseURL string
	client  *httpClient
}

func NewGeminiScanner(apiKey, model string, timeout time.Duration) (*GeminiScanner, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: GEMINI_API_KEY is empty", ErrNotConfigured)
	}

	if strings.TrimSpace(model) == "" {
		model = DefaultGeminiModel
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	return &GeminiScanner{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultGeminiBaseURL,
		client:  newHTTPClient(timeout),
	}, nil
}

func (s *GeminiScanner) Name() string { return "gemini:" + s.model }

type geminiInteractionRequest struct {
	Model             string                 `json:"model"`
	SystemInstruction string                 `json:"system_instruction"`
	Input             []geminiInputPart      `json:"input"`
	ResponseFormat    geminiResponseFormat   `json:"response_format"`
	GenerationConfig  geminiGenerationConfig `json:"generation_config"`
}

type geminiInputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type geminiResponseFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type"`
	Schema   map[string]any `json:"schema"`
}

// geminiGenerationConfig yalnızca düşünme seviyesini taşır. Örnekleme
// parametreleri bilinçli olarak gönderilmez (yukarıdaki model notu).
type geminiGenerationConfig struct {
	ThinkingLevel string `json:"thinking_level"`
}

type geminiInteraction struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Object string `json:"object"`
	Status string `json:"status"`
	Steps  []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"steps"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
	Usage struct {
		TotalInputTokens  int `json:"total_input_tokens"`
		TotalOutputTokens int `json:"total_output_tokens"`
	} `json:"usage"`
}

func (s *GeminiScanner) Scan(ctx context.Context, image Image) (ScanResult, error) {
	if len(image.Data) == 0 {
		return ScanResult{}, fmt.Errorf("%w: empty image", ErrMalformedImage)
	}

	request := geminiInteractionRequest{
		Model:             s.model,
		SystemInstruction: receiptPrompt,
		Input: []geminiInputPart{
			{Type: "text", Text: "Bu görseldeki fişi analiz et."},
			{
				Type:     "image",
				Data:     base64.StdEncoding.EncodeToString(image.Data),
				MimeType: image.MimeType,
			},
		},
		ResponseFormat: geminiResponseFormat{
			Type:     "text",
			MimeType: "application/json",
			Schema:   receiptResponseSchema(),
		},
		GenerationConfig: geminiGenerationConfig{ThinkingLevel: "low"},
	}

	var response geminiInteraction
	endpoint := strings.TrimRight(s.baseURL, "/") + "/v1beta/interactions"

	if err := s.client.postJSON(ctx, endpoint, map[string]string{
		"x-goog-api-key": s.apiKey,
	}, request, &response); err != nil {
		return ScanResult{}, err
	}

	if response.Error != nil {
		return ScanResult{}, fmt.Errorf("%w: %s", ErrUpstream, response.Error.Message)
	}
	// Etkileşim "completed" dışında bir durumda kaldıysa (ör. MAX_TOKENS
	// nedeniyle yarıda kesilme) çıktı güvenilmezdir.
	if response.Status != "" && response.Status != "completed" {
		return ScanResult{}, fmt.Errorf("%w: interaction finished with status %q", ErrMalformedResponse, response.Status)
	}

	text := extractGeminiText(response)
	if text == "" {
		return ScanResult{}, fmt.Errorf("%w: no text content in response", ErrMalformedResponse)
	}

	return ParseScanResponse(text)
}

// extractGeminiText, model çıktısını steps[].content[] içinden toplar.
// Düşünme ("thought") blokları atlanır; yalnızca type "text" olanlar alınır.
func extractGeminiText(response geminiInteraction) string {
	var parts []string

	for _, step := range response.Steps {
		if step.Type != "" && step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
				continue
			}
			parts = append(parts, content.Text)
		}
	}

	return strings.TrimSpace(strings.Join(parts, ""))
}

var _ Scanner = (*GeminiScanner)(nil)
