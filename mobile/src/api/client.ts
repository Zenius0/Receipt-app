import { env } from '../config/env';
import type {
  AuthUser,
  ConfirmReceiptPayload,
  Receipt,
  ReceiptCategory,
  ReceiptPage,
  ScanDraft,
  Session,
  SignInPayload,
} from './types';

const JSON_HEADERS = { 'Content-Type': 'application/json' } as const;

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  /** Günlük tarama kotası doldu mu? */
  get isQuotaExceeded(): boolean {
    return this.status === 429 && this.code === 'quota_exceeded';
  }

  /** Görsel bir fiş değil mi? */
  get isNotAReceipt(): boolean {
    return this.status === 422 && this.code === 'not_a_receipt';
  }

  /** Oturum geçersiz mi (süresi dolmuş veya kullanıcı silinmiş)? */
  get isUnauthorized(): boolean {
    return this.status === 401;
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  body?: unknown;
  token?: string | null;
  timeoutMs?: number;
}

interface ErrorBody {
  error?: string;
  message?: string;
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, token, timeoutMs = env.requestTimeoutMs } = options;

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(`${env.apiBaseUrl}${path}`, {
      method,
      headers: {
        ...JSON_HEADERS,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: controller.signal,
    });

    const payload = response.status === 204 ? null : await safeJson(response);

    if (!response.ok) {
      throw new ApiError(
        response.status,
        payload?.error ?? 'unknown_error',
        payload?.message ?? defaultMessageFor(response.status),
      );
    }

    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof Error && error.name === 'AbortError') {
      throw new ApiError(0, 'timeout', 'İstek zaman aşımına uğradı. İnternetinizi kontrol edin.');
    }
    throw new ApiError(0, 'network_error', 'Sunucuya ulaşılamadı. İnternet bağlantınızı kontrol edin.');
  } finally {
    clearTimeout(timeout);
  }
}

export const authApi = {
  signIn(provider: 'google' | 'apple', payload: SignInPayload) {
    return apiRequest<Session>(`/api/v1/auth/${provider}`, {
      method: 'POST',
      body: payload,
      timeoutMs: env.authTimeoutMs,
    });
  },

  me(token: string) {
    return apiRequest<AuthUser>('/api/v1/me', { token });
  },
};

export interface ReceiptListQuery {
  limit?: number;
  offset?: number;
  category?: ReceiptCategory;
  from?: string;
  to?: string;
}

export const receiptsApi = {
  /** Fiş görselini tarar. Görsel base64 olarak gönderilir. */
  scan(token: string, imageBase64: string, mimeType: string) {
    return apiRequest<ScanDraft>('/api/v1/receipts/scan', {
      method: 'POST',
      token,
      // AI çağrısı yavaş olabilir; genel istek zaman aşımından uzun tutulur.
      timeoutMs: env.scanTimeoutMs,
      body: { imageBase64: imageBase64, mimeType },
    });
  },

  confirm(token: string, payload: ConfirmReceiptPayload) {
    return apiRequest<Receipt>('/api/v1/receipts/confirm', {
      method: 'POST',
      token,
      body: payload,
    });
  },

  list(token: string, query: ReceiptListQuery = {}) {
    const params = new URLSearchParams();
    if (query.limit) params.set('limit', String(query.limit));
    if (query.offset) params.set('offset', String(query.offset));
    if (query.category) params.set('category', query.category);
    if (query.from) params.set('from', query.from);
    if (query.to) params.set('to', query.to);

    const qs = params.toString();
    return apiRequest<ReceiptPage>(`/api/v1/receipts${qs ? `?${qs}` : ''}`, { token });
  },
};

async function safeJson(response: Response): Promise<ErrorBody | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

/** Backend mesajı yoksa kullanıcıya anlamlı bir metin gösterir. */
function defaultMessageFor(status: number): string {
  if (status === 401) return 'Oturumunuz sona erdi. Lütfen tekrar giriş yapın.';
  if (status === 429) return 'Çok fazla istek gönderildi. Lütfen biraz bekleyin.';
  if (status === 502 || status === 504) return 'Servis şu anda yanıt vermiyor. Biraz sonra deneyin.';
  if (status >= 500) return 'Beklenmeyen bir hata oluştu.';
  return 'İstek başarısız oldu.';
}