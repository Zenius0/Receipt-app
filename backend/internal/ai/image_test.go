package ai

import (
	"encoding/base64"
	"errors"
	"testing"
)

// 1x1 saydam PNG.
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
		ok   bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, "image/jpeg", true},
		{"png", tinyPNG, "image/png", true},
		{"webp", append([]byte("RIFF"), []byte{0x10, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}...), "image/webp", true},
		{"heic", []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}, "image/heic", true},
		{"gif is not supported", []byte("GIF89a"), "", false},
		{"pdf is not supported", []byte("%PDF-1.7"), "", false},
		{"empty", nil, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DetectMimeType(tc.data)
			if got != tc.want || ok != tc.ok {
				t.Errorf("DetectMimeType = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDecodeBase64Image(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(tinyPNG)

	t.Run("plain base64", func(t *testing.T) {
		image, err := DecodeBase64Image(encoded, MaxImageBytes)
		if err != nil {
			t.Fatalf("DecodeBase64Image: %v", err)
		}
		if image.MimeType != "image/png" {
			t.Errorf("MimeType = %q, want image/png", image.MimeType)
		}
	})

	t.Run("data url", func(t *testing.T) {
		image, err := DecodeBase64Image("data:image/png;base64,"+encoded, MaxImageBytes)
		if err != nil {
			t.Fatalf("DecodeBase64Image: %v", err)
		}
		if len(image.Data) != len(tinyPNG) {
			t.Errorf("len(Data) = %d, want %d", len(image.Data), len(tinyPNG))
		}
	})

	t.Run("line breaks are tolerated", func(t *testing.T) {
		wrapped := encoded[:10] + "\n" + encoded[10:]
		if _, err := DecodeBase64Image(wrapped, MaxImageBytes); err != nil {
			t.Fatalf("DecodeBase64Image: %v", err)
		}
	})
}

func TestDecodeBase64Image_Errors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"empty", "", ErrMalformedImage},
		{"not base64", "!!!not base64!!!", ErrMalformedImage},
		{"not an image", base64.StdEncoding.EncodeToString([]byte("hello world, plain text")), ErrUnsupportedImage},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeBase64Image(tc.raw, MaxImageBytes)
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDecodeBase64Image_SizeLimit(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(tinyPNG)

	// 1 bayt bile sığmayan limit: base64 yükü çözülmeden elenmelidir.
	if _, err := DecodeBase64Image(encoded, 1); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("error = %v, want ErrImageTooLarge", err)
	}
}

func TestValidateImage_RejectsTruncatedPNG(t *testing.T) {
	// İmza doğru ama PNG verisi bozuk: AI'ya gönderilmeden elenmelidir.
	truncated := tinyPNG[:16]

	if _, err := ValidateImage(truncated, MaxImageBytes); !errors.Is(err, ErrMalformedImage) {
		t.Errorf("error = %v, want ErrMalformedImage", err)
	}
}
