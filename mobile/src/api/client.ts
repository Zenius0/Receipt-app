import { env } from '../config/env';
import type { AuthUser, Session, SignInPayload } from './types';

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
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  body?: unknown;
  token?: string | null;
  timeoutMs?: number;
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
      throw new ApiError(response.status, payload?.error ?? 'unknown_error', payload?.message ?? response.statusText);
    }

    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) {
      throw error;
    }
    if (error instanceof Error && error.name === 'AbortError') {
      throw new ApiError(0, 'timeout', 'request timed out');
    }
    throw new ApiError(0, 'network_error', 'sunucuya ulaşılamadı');
  } finally {
    clearTimeout(timeout);
  }
}

export const authApi = {
  signIn(provider: 'google' | 'apple', payload: SignInPayload) {
    return apiRequest<Session>(`/api/v1/auth/${provider}`, {
      method: 'POST',
      body: payload,
    });
  },

  me(token: string) {
    return apiRequest<AuthUser>('/api/v1/me', { token });
  },
};

async function safeJson(response: Response): Promise<ApiErrorBody | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}