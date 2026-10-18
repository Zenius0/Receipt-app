// Package ratelimit, istek sayısını sınırlayan anahtar bazlı sayaçlar sunar.
//
// İki ayrı amaç için kullanılır:
//
//   - Genel API limiti: istemci IP'si başına dakikada N istek. Kötüye
//     kullanımı ve boşa tekrar eden istekleri engeller.
//   - Tarama kotası: kullanıcı başına günde N fiş taraması. Yapay zekâ
//     çağrısı ücretli olduğu için asıl maliyet koruması budur.
//
// Tasarım notu: Bellek içi çalışır ve TEK sunucu örneği varsayar. Render'da
// birden fazla örneğe ölçeklenirse sayaçlar örnekler arasında paylaşılmaz ve
// gerçek limit N × örnek sayısı olur. Kalıcı kota (Redis) bu ADIM'ın kapsamı
// dışındadır; limit değerleri bu durumda düşük tutulmalıdır.
package ratelimit

import (
	"sync"
	"time"
)

// Result, bir anahtar için limit kontrolünün sonucunu taşır.
type Result struct {
	// Allowed, isteğin geçip geçmediği.
	Allowed bool
	// Remaining, pencerede kalan istek hakkı.
	Remaining int
	// Limit, pencereye ait toplam hak.
	Limit int
	// ResetAt, sayacın sıfırlanacağı an.
	ResetAt time.Time
	// RetryAfter, reddedilen istekte istemcinin beklemesi gereken süre.
	RetryAfter time.Duration
}

type bucket struct {
	count int
	reset time.Time
}

// Limiter, sabit pencere (fixed window) sayacıdır. Basit, bellek içi ve
// tek örnekli dağıtımla tam uyumludur.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	// sweepInterval, kaç anahtarın temizleneceğini belirler; bellek
	// sızmasını engeller.
	sweepInterval time.Duration
	lastSweep     time.Time
}

// New, limit > 0 olduğu sürece geçerli bir Limiter döndürür.
// limit <= 0 veya window <= 0 ise nil döner ve çağıran taraf limitin
// kapalı olduğunu nil kontrolüyle anlar.
func New(limit int, window time.Duration) *Limiter {
	if limit <= 0 || window <= 0 {
		return nil
	}

	return &Limiter{
		limit:         limit,
		window:        window,
		now:           time.Now,
		buckets:       make(map[string]*bucket),
		sweepInterval: window,
	}
}

// SetClock, testlerde zaman kaynağını değiştirir.
func (l *Limiter) SetClock(now func() time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// Allow, anahtar için bir isteği kaydeder ve geçip geçmediğini döner.
func (l *Limiter) Allow(key string) Result {
	if l == nil {
		// Limit kapalı: her istek geçer.
		return Result{Allowed: true}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	b, ok := l.buckets[key]
	if !ok || now.After(b.reset) {
		b = &bucket{count: 0, reset: now.Add(l.window)}
		l.buckets[key] = b
	}

	if b.count >= l.limit {
		return Result{
			Allowed:    false,
			Remaining:  0,
			Limit:      l.limit,
			ResetAt:    b.reset,
			RetryAfter: b.reset.Sub(now),
		}
	}

	b.count++

	return Result{
		Allowed:   true,
		Remaining: l.limit - b.count,
		Limit:     l.limit,
		ResetAt:   b.reset,
	}
}

// Reset, anahtarın sayacını siler. Kullanıcı çıkış yaptığında çağrılabilir.
func (l *Limiter) Reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// sweepLocked, süresi dolmuş anahtarları siler.
// Tam pencere dolmadan temizleme yapılırsa pencere kısalır; bu yüzden
// süresi dolmuş (now > reset) olanlar hedeflenir.
func (l *Limiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.sweepInterval {
		return
	}
	l.lastSweep = now

	for key, b := range l.buckets {
		if now.After(b.reset) {
			delete(l.buckets, key)
		}
	}
}

// Len, takip edilen anahtar sayısını döner (testler ve gözlem için).
func (l *Limiter) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
