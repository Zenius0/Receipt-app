const rawBaseUrl = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export const env = {
  apiBaseUrl: rawBaseUrl.replace(/\/+$/, ''),
  requestTimeoutMs: Number(process.env.EXPO_PUBLIC_REQUEST_TIMEOUT_MS ?? 15000),
  googleWebClientId: process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID ?? '',
  googleIosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID ?? '',
  sessionStorageKey: 'fatura.session.v1',
} as const;