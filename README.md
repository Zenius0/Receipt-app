# Akıllı Fatura ve Gider Takipçisi — Auth + Fiş Okuma Altyapısı

**ADIM 1 — Kimlik doğrulama:** Harici auth sağlayıcı kullanmadan, cihaz içi Google/Apple Sign-In
token'larını Go backend'de doğrulayan ve kendi oturum JWT'sini üreten katman.

**ADIM 2 — Veri tabanı ve çekirdek API:** Render PostgreSQL'de `receipts` tablosu, fiş görselini
yapay zekâya gönderip yapılandırılmış veri çıkaran `POST /api/v1/receipts/scan` ve kullanıcı
onayından sonra kaydı yazan `POST /api/v1/receipts/confirm`.

> **Maliyet ve gizlilik tasarımı:** Fiş görselleri sunucuda saklanmaz. Görsel cihazın yerel
> dosya sisteminde kalır; backend yalnızca metinsel metadata'yı ve cihaz içi dosya yolunu
> (`local_image_path`) saklar. Görsel AI'ya işlenmek için gönderilir, ancak kalıcılaştırılmaz.

## Mimari

```
┌─────────────────────────┐
│  Mobil (Expo / RN)      │
│                         │
│  @react-native-google-  │   id_token / identityToken
│  signin/google-signin   ├──────────────────────────┐
│  @invertase/react-native│                          │
│  -apple-authentication │
└─────────────────────────┘                          ▼
                                    ┌───────────────────────────────┐
                                    │  Go API (Render)              │
                                    │                               │
                                    │  POST /api/v1/auth/google     │
                                    │    └─ Google JWKS (RS256)      │
                                    │  POST /api/v1/auth/apple      │
                                    │    └─ Apple JWKS (RS256)      │
                                    │           │                   │
                                    │           ▼                   │
                                    │     auth.Service              │
                                    │       ├─ UserRepository       │
                                    │       └─ TokenIssuer (HS256)  │
                                    │           │                   │
                                    │  GET /api/v1/me (JWT guard)   │
                                    └───────────┬───────────────────┘
                                                ▼
                                    ┌───────────────────────┐
                                    │ PostgreSQL (Render)   │
                                    │ users                 │
                                    └───────────────────────┘
```

Akış: cihazdaki kütüphane token'ı üretir → backend token'ı sağlayıcıya doğrulatır →
kullanıcıyı `users` tablosuna upsert eder → backend kendi imzaladığı JWT'yi döner →
mobil oturumu `expo-secure-store` içinde saklar → sonraki istekler `Authorization: Bearer` ile korunur.

```
             ADIM 2 — Fiş akışı

 ┌─────────────────────────┐
 │  Mobil cihaz            │
 │                         │
 │  kamera / galeri ───────┼──► base64 veya multipart ──┐
 │  görsel: yerel dosya    │                             │
 │  (sunucuya kaydedilmez) │                             ▼
 └─────────────────────────┘           ┌────────────────────────────────┐
                                       │  POST /api/v1/receipts/scan   │
                                       │    └─ JWT guard                │
                                       │    └─ görsel doğrulama (mime,  │
                                       │       boyut, decode)           │
                                       │           │                   │
                                       │           ▼                   │
                                       │     ai.Scanner                │
                                       │       ├─ Gemini (varsayılan)  │
                                       │       └─ OpenAI (alternatif)  │
                                       │           │                   │
                                       │           ▼                   │
                                       │  JSON Schema ile zorlanmış   │
                                       │  çıktı + alan doğrulaması     │
                                       │           │                   │
                                       │           ▼                   │
                                       │  200 { storeName, totalAmount,│
                                       │       currency, category,     │
                                       │       receiptDate }           │
                                       │  (kullanıcı onayı bekler)     │
                                       └───────────┬───────────────────┘
                                                   │ POST /receipts/confirm
                                                   ▼
                                       ┌────────────────────────────────┐
                                       │  receipts (metadata + cihaz    │
                                       │  içi görsel yolu)               │
                                       └────────────────────────────────┘
```

`scan` isteğinde görsel belleğe alınır, AI'ya gönderilir ve istek sonunda düşer.
`confirm` isteğinde görsel tekrar gönderilmez — yalnızca cihazdaki dosyanın yolu kaydedilir.

## Proje Yapısı

```
.
├── db/schema.sql                      # bağımsız şema referansı
├── render.yaml                        # Render Blueprint (web + postgres)
├── backend/
│   ├── Dockerfile
│   ├── go.mod
│   ├── cmd/api/main.go                # wiring, graceful shutdown
│   └── internal/
│       ├── auth/                      # domain: provider doğrulama + oturum token'ı
│       │   ├── verifier.go            #   Verifier arayüzü + Identity
│       │   ├── google.go              #   Google JWKS (RS256) doğrulama
│       │   ├── apple.go               #   Apple JWKS + nonce doğrulama
│       │   ├── tokens.go              #   kendi JWT'mizi üretir/parse eder
│       │   └── service.go             #   SignIn orkestrasyonu
│       ├── config/config.go           # env doğrulamalı konfigürasyon
│       ├── httpx/response.go          # JSON yanıt yardımcıları
│       ├── httpapi/
│       │   ├── router.go              # route + middleware zinciri
│       │   └── handlers/
│       │       ├── auth.go            #   giriş ve /me handler'ları
│       │       └── receipts.go        #   scan (multipart/base64) + confirm
│       ├── ai/                        # ADIM 2: fiş görseli okuma
│       │   ├── scanner.go            #   Scanner arayüzü, prompt, JSON şema, doğrulama
│       │   ├── image.go              #   base64 çözme, mime tespiti, boyut kontrolü
│       │   ├── client.go             #   ortak HTTP istemcisi + yeniden deneme
│       │   ├── gemini.go             #   Gemini Interactions API
│       │   └── openai.go             #   OpenAI Chat Completions (alternatif)
│       ├── receipts/service.go        #   Scan, Confirm ve List orkestrasyonu
│       ├── middleware/                # JWT guard, CORS, log, recover, request-id
│       ├── platform/db/               # pgxpool + gömülü migration çalıştırıcı
│       │   └── migrations/
│       │       ├── 000001_create_users.sql
│       │       └── 000002_create_receipts.sql
│       └── store/                     # domain tipleri + repository arayüzleri
│           ├── store.go               #   UserRepository
│           ├── receipt.go             #   Receipt, filtre, kategori normalizasyonu
│           └── pgstore/               #   PostgreSQL implementasyonları
└── mobile/src/
    ├── config/env.ts
    ├── api/client.ts                  # fetch sarmalayıcı (timeout, bearer, hata eşleme)
    ├── api/types.ts
    ├── services/authService.ts        # Google/Apple token → backend
    └── hooks/useSession.ts            # React entegrasyon örneği
```

## Veritabanı

`users` tablosu `backend/internal/platform/db/migrations/000001_create_users.sql` içindedir ve
servis başlarken otomatik çalışır (`schema_migrations` ile idempotent).

| Kolon | Tip | Not |
|---|---|---|
| `id` | UUID | `gen_random_uuid()` PK |
| `email` | TEXT | unique, DB seviyesinde lowercase kontrolü |
| `name` | TEXT | nullable |
| `provider` | TEXT | `google` \| `apple` (CHECK) |
| `provider_user_id` | TEXT | sağlayıcının `sub` değeri, `(provider, provider_user_id)` unique |
| `accountant_email` | TEXT | nullable, kısmi index ile |
| `created_at` | TIMESTAMPTZ | `now()` |

Manuel kurulum isterseniz `db/schema.sql` dosyasını doğrudan çalıştırabilirsiniz.

### `receipts` tablosu (ADIM 2)

Migration: `backend/internal/platform/db/migrations/000002_create_receipts.sql`

| Kolon | Tip | Not |
|---|---|---|
| `id` | UUID | `gen_random_uuid()` PK |
| `user_id` | UUID | `users(id)` FK, `ON DELETE CASCADE` |
| `store_name` | TEXT | 1–200 karakter (CHECK) |
| `total_amount` | NUMERIC(12,2) | `>= 0` (CHECK) |
| `currency` | VARCHAR(3) | büyük harf ISO 4217 (CHECK) |
| `category` | TEXT | CHECK ile 12 kanonik değer |
| `receipt_date` | TIMESTAMPTZ | gelecek tarire izin verilmez (CHECK) |
| `local_image_path` | TEXT | nullable, **cihaz içi yol** — sunucuda dosya yok |
| `created_at` | TIMESTAMPTZ | `now()` |

İndeksler: `(user_id, created_at DESC)`, `(user_id, receipt_date DESC)`, `(user_id, category)`.

Kategori değerleri (DB kısıtı, AI prompt şeması ve Go sabitleri tek kaynaktan beslenir):
`fuel`, `food`, `groceries`, `repair`, `software`, `office`, `travel`, `health`,
`entertainment`, `clothing`, `utilities`, `other`.

> **Tasarım notu — `local_image_path`:** Bu değer sunucudan erişilebilir bir dosya yolu
> **değildir**; cihazın kendi dosya sisteminde çözümlenir. Kullanıcı cihaz değiştirdiğinde
> veya uygulamayı yeniden kurduğunda eski yollar geçersizleşir. Yalnızca metadata güvenilirdir.

## Endpoint'ler

### `POST /api/v1/auth/google`

```json
{ "token": "<google id_token>", "name": "Selim Aydın" }
```

### `POST /api/v1/auth/apple`

```json
{ "token": "<apple identityToken>", "name": "Selim Aydın", "nonce": "<cıhazın ürettiği ham nonce>" }
```

`name` opsiyoneldir; verilmezse token içindeki claim kullanılır. Apple `fullName` bilgisini yalnızca
ilk yetkilendirmede döndüğü için istemcinin adı buradan iletmesi gerekir.

Başarılı yanıt (`200`):

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "tokenType": "Bearer",
  "expiresAt": "2026-10-18T12:00:00Z",
  "user": {
    "id": "0f5c...",
    "email": "selim@example.com",
    "name": "Selim Aydın",
    "provider": "google",
    "accountantEmail": null,
    "createdAt": "2026-10-17T12:00:00Z"
  }
}
```

Hata gövdesi: `{"error": "<code>", "message": "<insan okunabilir>"}`

| Durum | `error` | Anlam |
|---|---|---|
| 400 | `invalid_request` | gövde bozuk / `token` eksik |
| 401 | `invalid_token` | sağlayıcı token'ı geçersiz, süresi dolmuş veya `aud`/`iss` eşleşmiyor |
| 401 | `email_not_verified` | Google tarafında e-posta doğrulanmamış |
| 409 | `email_conflict` | e-posta başka bir sağlayıcıyla kayıtlı |
| 500 | `internal_error` | beklenmeyen hata |

### `GET /api/v1/me` (korumalı)

`Authorization: Bearer <token>` olmadan `401` döner; token geçerliyse güncel kullanıcıyı döner.

### `POST /api/v1/receipts/scan` (korumalı)

Fiş görselini alır, yapay zekâya gönderir ve **kullanıcı onayı için** ayrıştırılmış veriyi
döner. Bu aşamada hiçbir şey veritabanına yazılmaz.

İki gövde biçimi desteklenir:

**a) multipart/form-data** — alan adı `image` (`file`, `receipt`, `photo` da kabul edilir):

```bash
curl -X POST https://<api>/api/v1/receipts/scan \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@receipt.jpg"
```

**b) application/json** — base64 gövde (data URL öneki de kabul edilir):

```json
{ "imageBase64": "data:image/jpeg;base64,/9j/4AAQ...", "mimeType": "image/jpeg" }
```

Başarılı yanıt (`200`):

```json
{
  "storeName": "Shell Petrol",
  "totalAmount": 1500.25,
  "currency": "TRY",
  "category": "fuel",
  "receiptDate": "2026-10-16T12:00:00Z",
  "taxNumber": "1234567890"
}
```

Yanıt başlığında `X-AI-Model` hangi sağlayıcı/modelin çalıştığını belirtir.

| Durum | `error` | Anlam |
|---|---|---|
| 400 | `invalid_image` | gövde bozuk, multipart alanı yok, base64 geçersiz |
| 413 | `image_too_large` | görsel `MAX_SCAN_IMAGE_BYTES` sınırını aşıyor |
| 415 | `unsupported_image` | JPEG/PNG/WEBP/HEIC dışında format veya desteklenmeyen `Content-Type` |
| 422 | `not_a_receipt` | görselde okunabilir bir fiş yok (`message` modelin açıklamasıdır) |
| 422 | `incomplete_receipt` | fiş geçerli ama zorunlu alan okunamadı |
| 429 | `rate_limited` | sağlayıcı kotası doldu |
| 502 | `ai_unavailable` | sağlayıcı 5xx döndürdü veya kullanılamaz yanıt verdi |
| 504 | `ai_timeout` | AI çağrısı zaman aşımına uğradı |
| 500 | `ai_not_configured` | sunucuda API anahtarı eksik (sunucu tarafı hata) |

### `GET /api/v1/receipts` (korumalı)

Kullanıcının fişlerini **en yeni önce** listeler. `user_id` JWT'den çözülür;
başka kullanıcının fişleri sorguya hiç girmez.

| Query | Açıklama |
|---|---|
| `limit` | Sayfa boyutu. Varsayılan `50`, en fazla `100`. |
| `offset` | Sayfa başlangıcı. Varsayılan `0`. |
| `category` | `fuel`, `food`, `groceries`, ... Türkçe karşılıkları da kabul edilir (`yakit`). |
| `from` | `YYYY-MM-DD` veya RFC3339. Fiş tarihi bu tarihten eski olamaz. |
| `to` | Aynı format. **Gün sonu dahildir.** |

```bash
curl "https://receipt-app-udje.onrender.com/api/v1/receipts?limit=20&category=fuel" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "items": [
    {
      "id": "6f1e...",
      "userId": "0f5c...",
      "storeName": "Shell Petrol",
      "totalAmount": 1500.25,
      "currency": "TRY",
      "category": "fuel",
      "receiptDate": "2026-10-16T12:00:00Z",
      "localImagePath": "file:///data/user/0/com.fatura/cache/receipt-1.jpg",
      "createdAt": "2026-10-17T12:03:00Z"
    }
  ],
  "nextOffset": 20,
  "total": 20
}
```

`nextOffset` **yoksa** (JSON'da görünmez) liste bitti demektir — istemci
`nextOffset` değerini bir sonraki istekte `offset` olarak gönderir.

| Durum | `error` | Anlam |
|---|---|---|
| 400 | `invalid_request` | `limit`/`offset` sayı değil, tarih geçersiz, `from > to` |
| 401 | `unauthorized` | oturum yok |
| 422 | `invalid_filter` | bilinmeyen kategori |

Boş sonuç `null` değil `{"items":[], "total":0}` döner; mobil istemci ayrım
kontrolü yapmak zorunda kalmaz.

### `POST /api/v1/receipts/confirm` (korumalı)

Kullanıcının onayladığı veriyi `receipts` tablosuna yazar. `user_id` **istemciden alınmaz**;
JWT'den çözülür.

```json
{
  "storeName": "Shell Petrol",
  "totalAmount": 1500.25,
  "currency": "TRY",
  "category": "fuel",
  "receiptDate": "2026-10-16T12:00:00Z",
  "localImagePath": "file:///data/user/0/com.fatura/cache/receipt-1.jpg"
}
```

Başarılı yanıt (`201`) — kaydedilen satır:

```json
{
  "id": "6f1e...",
  "userId": "0f5c...",
  "storeName": "Shell Petrol",
  "totalAmount": 1500.25,
  "currency": "TRY",
  "category": "fuel",
  "receiptDate": "2026-10-16T12:00:00Z",
  "localImagePath": "file:///data/user/0/com.fatura/cache/receipt-1.jpg",
  "createdAt": "2026-10-17T12:03:00Z"
}
```

| Durum | `error` | Anlam |
|---|---|---|
| 400 | `invalid_request` | gövde bozuk veya `receiptDate` RFC3339 değil |
| 401 | `unauthorized` | oturum yok veya kullanıcı satırı silinmiş |
| 422 | `invalid_receipt` | alan doğrulaması başarısız (negatif tutar, bilinmeyen kategori, gelecek tarih vb.) |

### Sağlık uçları

`GET /healthz` (liveness) ve `GET /readyz` (veritabanı erişilebilirliği, Render health check için `/healthz`).

## Yapay Zekâ Servisi

`AI_PROVIDER` ile iki sağlayıcı desteklenir. Varsayılan `gemini`'dir.

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `AI_PROVIDER` | `gemini` | `gemini` veya `openai` |
| `GEMINI_API_KEY` | — | `AI_PROVIDER=gemini` iken **zorunlu** |
| `GEMINI_MODEL` | `gemini-3.8-flash` | Flash katmanı fiyat/performans dengesi için tercih edilir |
| `OPENAI_API_KEY` | — | `AI_PROVIDER=openai` iken **zorunlu** |
| `OPENAI_MODEL` | `gpt-4o` | |
| `MAX_SCAN_IMAGE_BYTES` | `8388608` (8 MB) | gövde ve görsel sınırı |
| `SCAN_TIMEOUT` | `45s` | AI çağrısı zaman aşımı |

Eksik anahtar sunucuyu **başlangıçta** durdurur (`config.Load` hata döner); üretimde ilk fiş
denemesinde 500 almak yerine deploy'un başarısız görünmesi tercih edilir.

**Gemini API notu:** Gemini, eski `:generateContent` uç noktası yerine
`https://generativelanguage.googleapis.com/v1beta/interactions` uç noktasını kullanır ve
model çıktısı `steps[].content[]` içinde döner. Ayrıca Gemini 3.6+ modeller `temperature`,
`top_p` ve `top_k` değerlerini yok sayıp uyarı başlığı döndürür; bu yüzden istekte hiç
örnekleme parametresi gönderilmez, determinizm JSON Schema'dan gelir.

**Çıkarım ve doğrulama akışı:**

1. Görsel doğrulaması — MIME tipi **baş baytlardan** belirlenir (istemcinin gönderdiği
   `Content-Type`'a güvenilmez), boyut sınırı uygulanır, JPEG/PNG için decode edilerek
   bozuk dosyalar elenir.
2. Prompt + JSON Schema — kategori enum'u Go sabitlerinden üretilir, bu yüzden prompt,
   uygulama ve veritabanı CHECK kısıtı ayrışamaz.
3. Toleranslı ayrıştırma — model alan adlarında `camelCase`/`snake_case`/`Is_Receipt`
   farkına takılabilir; anahtarlar normalize edilir. `"1.234,56"` gibi Avrupa biçimi
   tutarlar da kabul edilir.
4. Alan doğrulaması — tutar `>= 0`, para birimi ISO 4217, kategori kanonik listede,
   tarih makul aralıkta. **AI ne döndürürse döndürsün veritabanına yazılamaz.**

## Yerel Çalıştırma

```bash
cd backend
cp .env.example .env      # DATABASE_URL, JWT_SECRET ve GEMINI_API_KEY'i doldurun
go run ./cmd/api
```

```bash
go test ./...             # birim + handler entegrasyon testleri
go vet ./...

# Gerçek PostgreSQL'e karşı uçtan uca test (migration + INSERT + kısıtlar)
DATABASE_URL="postgres://user:pass@localhost:5432/fatura?sslmode=disable" \
  go test ./internal/httpapi/handlers -run Postgres -v
```

## Yerel Geliştirme

Günlük akış tek komut: **`make setup`** (bir kere), sonra **`make run`**.

```bash
make setup   # backend/.env üretir + Docker ile PostgreSQL'i başlatır
make run     # API'yi localhost:8080'de çalıştırır (migration otomatik)
```

`make run` ayrı bir terminalde açık kalsın. Mobil uygulama veya curl ile
`http://localhost:8080` adresine bağlanın.

```bash
curl http://localhost:8080/healthz   # {"status":"ok"}
curl http://localhost:8080/readyz    # {"status":"ready"}
```

### Tüm komutlar

| Komut | Ne yapar |
|---|---|
| `make help` | Komut listesi |
| `make setup` | Sıfırdan kurulum (`.env` + veritabanı) |
| `make run` | API'yi çalıştır |
| `make test` | Tüm testler |
| `make test-postgres` | Gerçek PostgreSQL'e karşı uçtan uca test |
| `make vet` / `make fmt` | Statik kontrol / biçimlendirme |
| `make check` | CI kontrolü (vet + gofmt + test) |
| `make db-shell` | `psql` ile veritabanına gir |
| `make db-logs` | PostgreSQL loglarını izle |
| `make db-down` | Veritabanını durdur (veriler korunur) |
| `make db-reset` | Veriyi sil, sıfırdan kur |

### Veritabanı

`docker-compose.yml` PostgreSQL'i **port 5433**'te açar (Homebrew ile kurulu
yerel postgres'iniz 5432'de çalışıyorsa çakışmaz). Migration'lar servis başlarken
otomatik çalışır — `users` ve `receipts` tablolarını elle kurmanıza gerek yok.

Docker kullanmak istemiyorsanız `backend/.env` içindeki `DATABASE_URL`'i kendi
PostgreSQL'inize göre ayarlamanız yeterli.

### Editörde çalıştırma

VS Code kullanıyorsanız terminal yerine **Tasks** ile de çalıştırabilirsiniz.
En pratik yol: VS Code'un entegre terminalinde `make run` — dosya değişikliği
algılandığında `go run` yeniden derler, ayrı bir build komutu gerekmez.

### Production'ı yerelde denetleme

Render'a push etmeden önce Docker imajını yerelde deneyebilirsiniz:

```bash
make build            # production binary (11 MB, statik)
cd backend && ./bin/api
```

## Render'a Deploy

1. Repo'yu GitHub'a gönderin.
2. Render Dashboard → **New → Blueprint** → repo'yu seçin; `render.yaml` web servisi ve
   PostgreSQL'i birlikte oluşturur.
3. Dashboard'da **GOOGLE_CLIENT_IDS**, **APPLE_CLIENT_IDS**, **ALLOWED_ORIGINS** ve
   **GEMINI_API_KEY** değerlerini girin. `DATABASE_URL` ve `JWT_SECRET` otomatik oluşturulur.
4. Google Cloud Console → OAuth 2.0 Client ID'lerinden **Web client ID** (iOS client ID) ve
   Apple Developer → **Services ID** değerlerini yukarıya yazın.
5. [Google AI Studio](https://aistudio.google.com/apikey) → **Create API key** → üretilen
   anahtarı `GEMINI_API_KEY` olarak yapıştırın. (Anahtarı Render'a yazmadan önce
   `GEMINI_API_KEY` değerini **Render Dashboard → Environment** ekranında girmeniz gerekir.)
6. Sağlayıcı panolarına backend URL'sini yetkili redirect/origin olarak ekleyin.
7. `receipts` migration'ı servis ilk açılışta otomatik çalışır; ek bir işlem gerekmez.

> Render'da `REQUEST_TIMEOUT` değerini yükseltmeniz **gerekmez**: sunucunun yazma zaman aşımı
> `SCAN_TIMEOUT + 5s` olarak otomatik hesaplanır (`config.EffectiveWriteTimeout`).
> Render free plan'ın idle timeout'u düşük olduğu için üretimde `starter` plan önerilir.

## Hız Limitleri

| Değişken | Varsayılan | Kapsam |
|---|---|---|
| `RATE_LIMIT_PER_MINUTE` | `120` | IP başına genel istek (kimlik doğrulama öncesi) |
| `SCAN_DAILY_LIMIT` | `50` | Kullanıcı başına günlük fiş taraması |

Günlük kota **JWT guard'ın içinde** ve AI çağrısından hemen önce çalışır; kota
aşıldığında yapay zekâ çağrısı yapılmaz, dolayısıyla ücret harcanmaz. Kota
dolduğunda `429 quota_exceeded` döner ve istekteki `Retry-After`,
`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` başlıkları
bulunur. `0` değeri limiti kapatır.

Sayaçlar bellek içidir ve tek sunucu örneği varsayar. Birden fazla örneğe
ölçeklenirse gerçek sınır `N × örnek sayısı` olur.

## Mobil Uygulama

Expo SDK 57 / React 19.2 / React Native 0.86.

```bash
cd mobile
cp .env.example .env      # API adresi ve Google Client ID'yi doldurun
npm install
npx expo start            # QR kod basar
```

> **Neden SDK 57, neden 58 değil?** Play Store ve App Store'daki Expo Go uygulaması
> tek bir SDK sürümünü taşır ve mağazaya yeni sürüm çıkana kadar geride kalır.
> Proje SDK 58 istediğinde Expo Go *"Project is incompatible with this version of
> Expo Go"* hatası verir. SDK 57, mağazadaki Expo Go'nun sürümüyle birebir
> eşleşir. Bu yüzden SDK sürümü mağazadaki Expo Go'ya göre belirlenir, `latest`
> etiketine göre değil.

### Giriş butonları platforma göre

| Platform | Gösterilen butonlar |
|---|---|
| Android | Yalnızca **Google ile devam et** |
| iOS | **Google ile devam et** + **Apple ile devam et** |

Apple ile giriş bir iOS sistem özelliğidir (`AuthenticationServices`); Android'de
çalışmaz. Bu yüzden Android'de Apple butonu hiç gösterilmez — çalışmayan bir
buton koymak yerine tek seçenek bırakmak daha temiz. Karar `App.tsx` içinde
`showApple = Platform.OS === 'ios'` ile verilir.

Butonlar Expo Go'da da görünür; nihai arayüz birebir aynı kalsın diye. Ancak
native modül yoksa giriş ekranının altında *"Giriş için geliştirme derlemesi
gerekli"* bilgi kutusu çıkar.

### Expo Go'da ne çalışır, ne çalışmaz

| Özellik | Expo Go | Geliştirme derlemesi |
|---|---|---|
| Arayüzü görmek, ekranlar arası gezinmek | ✅ | ✅ |
| Kamera / galeriden fiş çekme | ✅ | ✅ |
| Google girişi | ❌ | ✅ |
| Apple girişi | ❌ | ✅ |

**Kural: uygulama, Expo Go'da bulunmayabilecek hiçbir native modülü dosya
yüklenirken import etmez.** Hem `@react-native-google-signin/google-signin` hem
`@invertase/react-native-apple-authentication` native modüllerdir; statik import
yapıldığında modül bulunamayınca uygulama açılışta çöker ve Expo Go sessizce
kapanır (hata mesajı çıkmaz).

Tembel yükleme (`authService.loadGoogleSignin` / `loadAppleAuth`) tek başına
**yeterli değildir**: Metro dinamik `import()`'u ayrı bir chunk'a çıkarıyor ve
chunk değerlendirilirken patlayan hata promise zincirinin dışına kaçıyor, bu
yüzden `.catch()` yakalayamıyor. Doğru çözüm: **JS dosyasına dokunmadan önce**
native modülün varlığını sormak — `TurboModuleRegistry.get` hata fırlatmadan
`null` döner (`getEnforcing` fırlatır). Apple için karşılığı
`NativeModules.RNAppleAuthModule` / `RNAppleAuthModuleAndroid`.

### Bulut derlemesinde `.env` yüklenmez

EAS Build, projeyi `.gitignore` kurallarına göre paketler; `mobile/.env`
gitignore'da olduğu için **yüklenmez**. Bu yüzden `EXPO_PUBLIC_*` değişkenleri
build sırasında EAS tarafından sağlanmalıdır:

```bash
eas env:create --name EXPO_PUBLIC_API_URL \
  --value "https://receipt-app-udje.onrender.com" \
  --visibility plaintext --environment development --non-interactive
```

`development`, `preview` ve `production` ortamlarının üçüne de eklendi.

### `expo-dev-client` ve Expo Go birbirini dışlar

`expo-dev-client` kurulu olduğunda `npx expo start` artık Expo Go'ya değil,
senin derlemenin geliştirici sürümüne hizmet eder; Expo Go proje açılınca
kendini kapatır. Bu yüzden ikisi **aynı anda kullanılamaz**:

```bash
# Expo Go ile çalıştırmak için (arayüzü görmek)
npm uninstall expo-dev-client
npx expo start

# Geliştirme derlemesi hazırlamak için (Google/Apple girişi)
npx expo install expo-dev-client
eas build -p android --profile development
npx expo start --dev-client
```

### Google girişini denemek için geliştirme derlemesi

EAS CLI gerekli. Projeye bağımlılık olarak kurulmaz (Expo Doctor reddeder).
Önerilen yol — global kurulum:

```bash
sudo npm install -g eas-cli     # bir kez şifre sorar
eas --version                    # doğrulama
```

```bash
cd mobile
eas login                        # ücretsiz Expo hesabı
eas build -p android --profile development
```

> **`npx eas` yazmayın.** npm'de Expo ile ilgisi olmayan, aynı adlı bir `eas`
> paketi (0.1.0) var; `npx eas` onu indirir. Doğru ad `eas-cli`.
>
> Global kurulum yapamıyorsanız kurulumsuz da kullanılır, ama her komutta tam adı
> yazılır: `npx eas-cli login`, `npx eas-cli build -p android --profile development`

Bulunan APK'yı telefona kurun. Bundan sonra Expo Go'yu **kullanmayın**:

```bash
npx expo start --dev-client
```

Telefonda açılan listeden **Fatura Takipçi** uygulamasını seçin (Expo Go değil).

> `eas build` bulutta çalışır ve ücretsizdir; ilk Android derlemesi ~15 dakika
> sürer. Her native modül eklendiğinde yeniden derleme gerekir.

### Çalıştırma seçenekleri

| Yöntem | Ne zaman |
|---|---|
| **Expo Go + gerçek telefon** | Arayüzü görmek için en hızlı yol. Google girişi çalışmaz. |
| **Geliştirme derlemesi** | Google girişi dahil her şeyi denemek için. |
| **iOS Simulator** | macOS'ta Xcode yüklüyse `npx expo start --ios`. Kamera için "Photo Library" yükleme yeterli. |
| Android emülatör | Android Studio (~10 GB) gerektirir; gerek yok. |

### `EXPO_PUBLIC_API_URL` ayarı

Backend Render'da çalışıyorsa (önerilen):

```
EXPO_PUBLIC_API_URL=https://<render-servis-adresi>
```

Backend'i bilgisayarınızda `make run` ile çalıştırıyorsanız:

- iOS Simulator / web → `http://localhost:8080`
- **Gerçek telefon** → bilgisayarınızın LAN adresi: `ipconfig getifaddr en0`

> Render free plan 15 dakika hareketsizlikte servisi kapatır; ilk istek ~1 dakika
> sürebilir. Bu yüzden uygulamayı açarken "yükleniyor" ekranında biraz beklemek
> normaldir. `0.5c-512mb` planına geçilirse bu bekleme kaybolur.

**Giriş akışı:** `Giriş → Ana ekran → Fiş Tara → Kamera → Onay ekranı → Kaydet → Liste`

Tasarım kararı olarak AI çıktısı **asla doğrudan kaydedilmez**: kullanıcı
onay ekranında her alanı görebilir ve düzeltebilir. Görsel AI'a gönderilmeden
önce 1600 px'e küçültülür (3-5 MB → ~200 KB), hem yükleme hızı hem maliyet
düşer.

## Güvenlik Notları

- Sağlayıcı token'ları backend'de doğrulanır; istemciden gelen hiçbir `sub`/`email` değerine güvenilmez.
- Google için `iss`, `exp` ve `aud`/`azp` (izinli client ID listesi) kontrolü yapılır; `email_verified` zorunludur.
- **Google doğrulaması JWKS üzerinden yapılır** (`https://www.googleapis.com/oauth2/v3/certs`, RS256),
  `oauth2.googleapis.com/tokeninfo` üzerinden değil. Tokeninfo legacy bir uç noktadır ve mobil
  (native) istemciler için üretilen id_token'ları reddedebilir; mobil uygulamadan gelen token
  tam olarak bu türden olduğu için doğrulama başarısız oluyordu. Apple tarafındakiyle aynı yöntem
  hem sorunu kaldırır hem de token'ı doğrulamadan ağa göndermez.
- **Android'de Google girişi için** uygulamanın imza parmak izi (SHA-1) Google Cloud Console'da
  *Android istemcisi* olarak kayıtlı olmalıdır. Paket adı `com.selim.fatura`, SHA-1 ise EAS'in
  yönettiği keystore'undan gelir ve derlenen APK'nın imzasından okunabilir. Bu kayıt yoksa giriş
  "DEVELOPER_ERROR" ile başarısız olur.
- Reddedilen token'ın sebebi API yanıtının `message` alanında döner (örn. `audience ... is not
  allowed`); eski genel mesaj yerine bu sayede ayar hataları görünür olur.
- Apple için imza Apple JWKS ile doğrulanılır, `iss`, `aud` (bundle ID / Services ID), `exp` ve
  **nonce** eşleşmesi kontrol edilir; bilinmeyen `kid` geldiğinde anahtarlar otomatik yenilenir.
- Oturum JWT'si HS256 ile imzalanır; `JWT_SECRET` en az 32 bayt olmalı, prod'da Render tarafından üretilir.
- Middleware imza algoritmasını açıkça HS256 ile sınırlar (`alg: none` / algoritma karıştırma saldırısına kapalı).
- JSON gövde boyutu 16 KB ile sınırlı, bilinmeyen JSON alanları reddedilir.
- Fiş görselleri 8 MB ve 12 MB (base64 sarmalayıcı) ile sınırlı; MIME tipi istemciden değil
  dosyanın baş baytlarından belirlenir, AI'ya gönderilmeden decode edilerek bozuk dosyalar elenir.
- `receipts.user_id` JWT'den çözülür; istemcinin gönderdiği kullanıcı kimliği yok sayılır.
- AI çıktısı istemciye onay için döner, veritabanına doğrudan yazılmaz; `confirm` isteğinde
  alanlar yeniden doğrulanır (domain katmanı + veritabanı CHECK kısıtı).
- AI anahtarı yalnızca sunucu ortam değişkenlerinde tutulur, loglara yazılmaz.
- Sağlayıcı hatalarında 429 ve 5xx için üç denemeli kısa geri çekilme; 4xx'te tek deneme.
- Apple özel relay e-posta adresleri (`*@privaterelay.appleid.com`) normal kullanıcı e-postası gibi saklanır.

### Bilinen tasarım kararları

- `users.provider` tek kolon olduğu için bir e-posta hem Google hem Apple ile giriş yaparsa ikinci
  giriş `409 email_conflict` ile reddedilir. Çoklu sağlayıcı bağlama istenirse `provider` kolonu
  ayrı bir `user_identities` tablosuna taşınmalıdır; bu ADIM'ın kapsamı dışında tutuldu.
- **Görsel sunucuda saklanmıyor.** `local_image_path` cihaza özgüdür; cihaz değişirse
  geçersizleşir. Üçüncü taraf birden fazla cihazı desteklemek ise ayrı bir yükleme servisi
  (S3/R2 gibi) gerektirir ve ek maliyet getirir; bilinçli olarak bu ADIM'ın dışında bırakıldı.
- **`taxNumber` yalnızca scan yanıtında döner, `receipts` tablosunda saklanmaz.** Bu ADIM'ın
  tabloda istenen kolonlar listesinde yoktu; alanı sonradan eklemek `ALTER TABLE` gerektirir.
- **Tutar `float64` taşınır, `NUMERIC(12,2)` yazılır.** Fiş tutarları iki ondalıklı olduğu için
  bu güvenli kabul edilmiştir; hassas para aritmetiği gerekirse `shopspring/decimal` eklenmelidir.
- **Fiş tarihi gün ortasına sabitlenir** (12:00 UTC). Saat bilgisi yoksa gece yarısına
  yakın UTC dönüşümleri tarihi bir gün kaydırabiliyordu.
- **Tek tıklamalı kayıt yok.** `scan` ve `confirm` ayrıdır; AI çıkarımı ile kalıcı kayıt arasında
  kullanıcının onayı vardır. Bu, yanlış okunan fişlerin veritabanına girilmesini engeller.

## ADIM 3 için bırakılan noktalar

- Mobil taraf: `scanReceipt()` / `confirmReceipt()` / `listReceipts()` servisleri ve ekranları
- Aylık kategori dökümü için toplam (`total`) sayım ucu — indeksler hazır, sorgu eksik
- Fiş görseli kırpma/iyileştirme (AI'ya göndermeden önce EXIF temizleme, yeniden boyutlandırma)
- `accountant_email` üzerinden muhasebeci davet akışı
- Refresh token tablosu ve token yenileme (`POST /api/v1/auth/refresh`)
- Hız limiti (kullanıcı başına günlük tarama sayısı) — AI maliyetini korumak için