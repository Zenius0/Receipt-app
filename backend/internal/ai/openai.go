package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/selim/fatura-backend/internal/store"
)

// DefaultOpenAIModel, görsel + yapılandırılmış çıktıyı destekleyen model.
const DefaultOpenAIModel = "gpt-4o"

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIScanner, OpenAI Chat Completions API'sini konuşur. Gemini'ye alternatiftir;
// AI_PROVIDER=openai ile etkinleştirilir.
//
// Not: Gemini'ye göre burada temperature/top_p gönderilebilir; OpenAI bu
// parametreleri destekler ve JSON modunda daha kararlı sonuç verir.
type OpenAIScanner struct {
	apiKey  string
	model   string
	baseURL string
	client  *httpClient
}

func NewOpenAIScanner(apiKey, model string, timeout time.Duration) (*OpenAIScanner, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: OPENAI_API_KEY is empty", ErrNotConfigured)
	}

	if strings.TrimSpace(model) == "" {
		model = DefaultOpenAIModel
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	return &OpenAIScanner{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultOpenAIBaseURL,
		client:  newHTTPClient(timeout),
	}, nil
}

func (s *OpenAIScanner) Name() string { return "openai:" + s.model }

type openAIRequest struct {
	Model          string           `json:"model"`
	Temperature    float64          `json:"temperature"`
	MaxTokens      int              `json:"max_completion_tokens"`
	ResponseFormat openAIJSONFormat `json:"response_format"`
	Messages       []openAIMessage  `json:"messages"`
}

type openAIJSONFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAISchemaSpec `json:"json_schema,omitempty"`
}

type openAISchemaSpec struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAITextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIImagePart struct {
	Type     string         `json:"type"`
	ImageURL openAIImageURL `json:"image_url"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *apiError `json:"error"`
}

func (s *OpenAIScanner) Scan(ctx context.Context, image Image) (ScanResult, error) {
	if len(image.Data) == 0 {
		return ScanResult{}, fmt.Errorf("%w: empty image", ErrMalformedImage)
	}

	request := openAIRequest{
		Model:       s.model,
		Temperature: 0,
		MaxTokens:   800,
		ResponseFormat: openAIJSONFormat{
			Type:       "json_schema",
			JSONSchema: &openAISchemaSpec{Name: "receipt_extraction", Strict: true, Schema: openAIResponseSchema()},
		},
		Messages: []openAIMessage{
			{Role: "system", Content: receiptPrompt},
			{
				Role: "user",
				Content: []any{
					openAITextPart{Type: "text", Text: "Bu görseldeki fişi analiz et."},
					openAIImagePart{
						Type:     "image_url",
						ImageURL: openAIImageURL{URL: fmt.Sprintf("data:%s;base64,%s", image.MimeType, base64.StdEncoding.EncodeToString(image.Data))},
					},
				},
			},
		},
	}

	var response openAIResponse
	endpoint := strings.TrimRight(s.baseURL, "/") + "/chat/completions"

	if err := s.client.postJSON(ctx, endpoint, map[string]string{
		"Authorization": "Bearer " + s.apiKey,
	}, request, &response); err != nil {
		return ScanResult{}, err
	}

	if response.Error != nil {
		return ScanResult{}, fmt.Errorf("%w: %s", ErrUpstream, response.Error.String())
	}
	if len(response.Choices) == 0 {
		return ScanResult{}, fmt.Errorf("%w: no choices in response", ErrMalformedResponse)
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		// length / content_filter sonucu: model yanıt üretmeden kesti.
		return ScanResult{}, fmt.Errorf("%w: empty content (finish_reason=%s)", ErrMalformedResponse, response.Choices[0].FinishReason)
	}

	return ParseScanResponse(content)
}

// openAIResponseSchema, OpenAI strict modunda tüm alanların "required" listesinde
// bulunması zorunludur. Bu yüzden isteğe bağlı alanlar nullable tip olarak
// tanımlanır; alıcı tarafta null, ParseScanResponse içinde eksik alan gibi
// davranır.
func openAIResponseSchema() map[string]any {
	// Kategori enum'u store ile paylaşılan tek kaynaktan gelir.
	categoryEnum := make([]any, 0, len(store.AllCategories())+1)
	categoryEnum = append(categoryEnum, nil)
	for _, c := range store.AllCategories() {
		categoryEnum = append(categoryEnum, string(c))
	}

	currencyEnum := []any{nil, "TRY", "USD", "EUR", "GBP", "JPY"}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"isReceipt": map[string]any{"type": "boolean"},
			"reason":    map[string]any{"type": []string{"string", "null"}},
			"storeName": map[string]any{"type": []string{"string", "null"}},
			"totalAmount": map[string]any{
				"type": []string{"number", "null"},
			},
			"currency": map[string]any{
				"type": []string{"string", "null"},
				"enum": currencyEnum,
			},
			"category": map[string]any{
				"type": []string{"string", "null"},
				"enum": categoryEnum,
			},
			"receiptDate": map[string]any{"type": []string{"string", "null"}},
			"taxNumber":   map[string]any{"type": []string{"string", "null"}},
		},
		"required": []string{
			"isReceipt", "reason", "storeName", "totalAmount",
			"currency", "category", "receiptDate", "taxNumber",
		},
		"additionalProperties": false,
	}
}

var _ Scanner = (*OpenAIScanner)(nil)
