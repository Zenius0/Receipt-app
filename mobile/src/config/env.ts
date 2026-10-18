const rawBaseUrl = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export const env = {
  apiBaseUrl: rawBaseUrl.replace(/\/+$/, ''),

  /** Genel API istekleri için zaman aşımı. */
  requestTimeoutMs: Number(process.env.EXPO_PUBLIC_REQUEST_TIMEOUT_MS ?? 15000),

  /** Giriş akışı (token doğrulama) biraz daha uzun sürebilir. */
  authTimeoutMs: Number(process.env.EXPO_PUBLIC_AUTH_TIMEOUT_MS ?? 20000),

  /**
   * Fiş tarama isteği. Yapay zekâ çağrısı 10-30 saniye sürebilir; backend'in
   * SCAN_TIMEOUT değeri (45s) ile uyumlu tutulmalı.
   */
  scanTimeoutMs: Number(process.env.EXPO_PUBLIC_SCAN_TIMEOUT_MS ?? 60000),

  /**
   * AI'ya gönderilmeden önce görselin küçültüleceği maksimum genişlik.
   * Telefon fotoğrafları 3-5 MB olur; 1600px'e indirmek yükü ~200 KB'a
   * indirir, hem yükleme hızını hem de yapay zekâ maliyetini düşürür.
   */
  maxScanImageWidth: Number(process.env.EXPO_PUBLIC_MAX_SCAN_WIDTH ?? 1600),

  googleWebClientId: process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID ?? '',
  googleIosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID ?? '',
  sessionStorageKey: 'fatura.session.v1',
} as const;