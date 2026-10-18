package ratelimit

import (
	"sync"
	"testing"
	"time"
)

// testClock, ileri alınabilen sahte saat. Değişkeni yakalayan closure
// kullanan yardımcılar hatalıydı: test now'u ileri aldığında sayaç sabit
// kalıyor ve pencere sıfırlanması hiç test edilemiyordu.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(t time.Time) *testClock {
	return &testClock{now: t}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestLimiter_AllowsUpToLimit(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	limiter := New(3, time.Minute)
	clock := newTestClock(now)
	limiter.SetClock(clock.Now)

	for i := 0; i < 3; i++ {
		result := limiter.Allow("user-1")
		if !result.Allowed {
			t.Fatalf("istek %d reddedildi, ilk 3 geçmeliydi", i+1)
		}
		if want := 2 - i; result.Remaining != want {
			t.Errorf("istek %d Remaining = %d, want %d", i+1, result.Remaining, want)
		}
	}

	fourth := limiter.Allow("user-1")
	if fourth.Allowed {
		t.Fatal("4. istek geçti, limit 3'tü")
	}
	if fourth.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", fourth.Remaining)
	}
	if fourth.RetryAfter <= 0 {
		t.Error("RetryAfter pozitif olmalı")
	}
}

func TestLimiter_KeysAreIndependent(t *testing.T) {
	limiter := New(2, time.Minute)

	for i := 0; i < 2; i++ {
		if !limiter.Allow("user-a").Allowed {
			t.Fatal("user-a limiti dolmadan reddedildi")
		}
	}
	if limiter.Allow("user-a").Allowed {
		t.Fatal("user-a limiti doldu ama geçti")
	}

	// Kullanıcı B etkilenmemeli.
	if !limiter.Allow("user-b").Allowed {
		t.Error("user-a limiti user-b'yi etkiliyor")
	}
}

func TestLimiter_WindowResets(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute)
	clock := newTestClock(now)
	limiter.SetClock(clock.Now)

	limiter.Allow("user-1")
	limiter.Allow("user-1")
	if limiter.Allow("user-1").Allowed {
		t.Fatal("limit dolmadan geçti")
	}

	// Pencere dolmadan hâlâ reddedilmeli.
	clock.Advance(59 * time.Second)
	if limiter.Allow("user-1").Allowed {
		t.Error("pencere bitmeden sayac sıfırlandı")
	}

	// Pencere doldu.
	clock.Advance(2 * time.Second)
	result := limiter.Allow("user-1")
	if !result.Allowed {
		t.Fatal("pencere dolduğu için istek geçmeliydi")
	}
	if result.Remaining != 1 {
		t.Errorf("Remaining = %d, want 1 (pencere sıfırlandıktan sonra ilk istek)", result.Remaining)
	}
}

// ResetAt, istemcinin ne zaman tekrar deneyeceğini bildirmesi için gereklidir.
func TestLimiter_ResetAtIsCorrect(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	limiter := New(1, time.Hour)
	clock := newTestClock(now)
	limiter.SetClock(clock.Now)

	limiter.Allow("user-1")
	result := limiter.Allow("user-1")

	wantReset := now.Add(time.Hour)
	if !result.ResetAt.Equal(wantReset) {
		t.Errorf("ResetAt = %v, want %v", result.ResetAt, wantReset)
	}
	if result.RetryAfter != time.Hour {
		t.Errorf("RetryAfter = %v, want 1h", result.RetryAfter)
	}
}

// nil Limiter "limit kapalı" demektir: hiçbir istek reddedilmemeli.
// Bu, AI_PROVIDER kapalıyken veya limit 0 yapılandırıldığında korunur.
func TestLimiter_NilDisablesLimit(t *testing.T) {
	var limiter *Limiter

	for i := 0; i < 1000; i++ {
		if !limiter.Allow("user-1").Allowed {
			t.Fatalf("nil limiter %d. istekte reddetti", i)
		}
	}
	limiter.SetClock(time.Now)
	limiter.Reset("user-1")
	if limiter.Len() != 0 {
		t.Error("nil limiter Len() != 0")
	}
}

func TestNew_InvalidConfigDisables(t *testing.T) {
	if New(0, time.Minute) != nil {
		t.Error("limit 0 nil dönmeli")
	}
	if New(-1, time.Minute) != nil {
		t.Error("negatif limit nil dönmeli")
	}
	if New(5, 0) != nil {
		t.Error("window 0 nil dönmeli")
	}
}

// Süresi dolmuş anahtarlar silinmeli; aksi halde bellek sınırsız büyür.
func TestLimiter_SweepsExpiredBuckets(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	limiter := New(1, time.Minute)
	clock := newTestClock(now)
	limiter.SetClock(clock.Now)

	for i := 0; i < 100; i++ {
		limiter.Allow(string(rune('a' + i%26)))
	}
	limiter.Allow("tek-anahtar")

	if limiter.Len() < 2 {
		t.Fatalf("Len = %d, want >= 2", limiter.Len())
	}

	// Birden fazla pencere geçsin.
	clock.Advance(10 * time.Minute)
	limiter.Allow("tek-anahtar")

	if limiter.Len() != 1 {
		t.Errorf("Len = %d, want 1 (süresi dolanlar temizlenmeli)", limiter.Len())
	}
}

func TestLimiter_Reset(t *testing.T) {
	limiter := New(1, time.Minute)

	limiter.Allow("user-1")
	if limiter.Allow("user-1").Allowed {
		t.Fatal("limit doldu ama geçti")
	}

	limiter.Reset("user-1")
	if !limiter.Allow("user-1").Allowed {
		t.Error("Reset sonrası istek geçmeliydi")
	}
}

// Eşzamanlı isteklerde sayaç bozulmamalı; race detector bunu doğrular.
func TestLimiter_ConcurrentAccess(t *testing.T) {
	limiter := New(50, time.Minute)

	done := make(chan struct{})
	allowed := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			allowed <- limiter.Allow("user-1").Allowed
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	close(allowed)

	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}
	if count != 50 {
		t.Errorf("eşzamanlı isteklerde izin verilen = %d, want 50", count)
	}
}
