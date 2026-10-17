package ai

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // JPEG doğrulama için decoder kayıt altına alınır
	_ "image/png"  // PNG doğrulama için decoder kayıt altına alınır
	"strings"
)

// MaxImageBytes, tek bir fiş görseli için üst sınır (8 MB).
// Gemini'nin inline veri limiti 20 MB'dir; mobil cihazdan gelen telefon
// fotoğraflarının neredeyse tamamı bu sınırın içindedir.
const MaxImageBytes = 8 << 20

var (
	ErrImageTooLarge    = errors.New("ai: image is too large")
	ErrUnsupportedImage = errors.New("ai: unsupported image format")
	ErrMalformedImage   = errors.New("ai: image data is malformed")
)

// Image, doğrulanmış ve AI'ya gönderilmeye hazır bir fiş görselidir.
type Image struct {
	Data     []byte
	MimeType string
}

// DecodeBase64Image, istemciden gelen base64 metnini (data URL öneki olsun ya da
// olmasın) çözer ve doğrulanmış bir Image döndürür.
func DecodeBase64Image(raw string, maxBytes int) (Image, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return Image{}, fmt.Errorf("%w: empty payload", ErrMalformedImage)
	}

	// data:image/jpeg;base64,AAAA... biçimini de kabul et.
	if idx := strings.Index(payload, ","); idx >= 0 && strings.HasPrefix(strings.ToLower(payload), "data:") {
		payload = payload[idx+1:]
	}

	// Mobil istemciler satır sonu ekleyebiliyor; temizleyip kodlayalım.
	payload = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, payload)

	if len(payload) > maxDecodedLen(maxBytes) {
		return Image{}, fmt.Errorf("%w: base64 payload exceeds %d bytes decoded", ErrImageTooLarge, maxBytes)
	}

	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Dolgusuz (raw) base64 gönderen istemciler için tekrar dene.
		data, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return Image{}, fmt.Errorf("%w: not valid base64", ErrMalformedImage)
		}
	}

	return ValidateImage(data, maxBytes)
}

// ValidateImage, bayt dizisinin gerçekten desteklenen bir görsel olduğunu ve
// boyut sınırına uyduğunu doğrular.
func ValidateImage(data []byte, maxBytes int) (Image, error) {
	if len(data) == 0 {
		return Image{}, fmt.Errorf("%w: empty", ErrMalformedImage)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return Image{}, fmt.Errorf("%w: %d bytes exceeds limit of %d", ErrImageTooLarge, len(data), maxBytes)
	}

	mime, ok := DetectMimeType(data)
	if !ok {
		return Image{}, fmt.Errorf("%w: signature not recognised", ErrUnsupportedImage)
	}

	// JPEG/PNG için ek olarak gerçekten çözülebilirliğini doğrula; bozuk veya
	// yarım yüklenmiş dosyalar AI'ya gönderilmeden elenir.
	switch mime {
	case "image/jpeg", "image/png":
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return Image{}, fmt.Errorf("%w: %v", ErrMalformedImage, err)
		}
	}

	return Image{Data: data, MimeType: mime}, nil
}

// DetectMimeType, dosyanın baş baytlarından MIME tipini belirler. İstemciden
// gelen Content-Type'a güvenilmez; AI'ya yanlış mime_type göndermek 400 hatası
// üretir.
func DetectMimeType(data []byte) (string, bool) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", true
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", true
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", true
	case isHEIF(data):
		return "image/heic", true
	}
	return "", false
}

// isHEIF, ISO-BMFF kutu tipini (ftyp) ve marka alanını kontrol eder.
func isHEIF(data []byte) bool {
	if len(data) < 12 || !bytes.Equal(data[4:8], []byte("ftyp")) {
		return false
	}

	brand := string(data[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1":
		return true
	}
	return false
}

// maxDecodedLen, base64 metninin çözülmüş haldeki en büyük olası boyutunu
// döndürür; limiti aşan yüklemeleri belleğe almadan reddetmek için kullanılır.
func maxDecodedLen(maxBytes int) int {
	if maxBytes <= 0 {
		maxBytes = MaxImageBytes
	}
	return maxBytes/3*4 + 4
}
