import AppleAuthentication from '@invertase/react-native-apple-authentication';
import { GoogleSignin, isErrorWithCode, statusCodes } from '@react-native-google-signin/google-signin';
import * as Crypto from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

import { ApiError, authApi } from '../api/client';
import type { AuthUser, Session } from '../api/types';
import { env } from '../config/env';

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AuthError';
  }
}

let configured = false;

/**
 * Google girişini hazırlar.
 *
 * @react-native-google-signin/google-signin v16'da isimlendirilmiş fonksiyon
 * ihraçları kaldırıldı; tüm API tek bir GoogleSignin nesnesinde toplandı.
 */
function ensureGoogleConfigured(): void {
  if (configured) return;

  GoogleSignin.configure({
    // iOS ve Android aynı Web Client ID kullanır; mobil kütüphaneler
    // Google'ın önerdiği bu yaklaşımı uygular.
    webClientId: env.googleWebClientId,
    iosClientId: env.googleIosClientId || env.googleWebClientId,
    offlineAccess: false,
  });

  configured = true;
}

async function signInWithGoogleProvider(): Promise<Session> {
  if (!env.googleWebClientId && !env.googleIosClientId) {
    throw new AuthError(
      'Google girişi yapılandırılmamış. EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID değerini .env dosyanıza ekleyin.',
    );
  }

  ensureGoogleConfigured();

  try {
    await GoogleSignin.hasPlayServices();
  } catch {
    // hasPlayServices yalnızca Android'de anlamlıdır; yoksa sessizce geç.
  }

  const response = await GoogleSignin.signIn();

  if (response.type !== 'success') {
    if (response.type === 'cancelled') {
      throw new AuthError('Giriş iptal edildi.');
    }
    throw new AuthError('Google girişi tamamlanamadı.');
  }

  // v16 yanıtı doğrudan idToken taşıyor; sürüm farklılıklarında getTokens'e düşüyoruz.
  let idToken = response.data.idToken;
  if (!idToken) {
    idToken = (await GoogleSignin.getTokens()).idToken;
  }
  if (!idToken) {
    throw new AuthError('Google kimlik belirteci alınamadı.');
  }

  return authApi.signIn('google', {
    token: idToken,
    name: response.data.user?.name ?? undefined,
  });
}

async function signInWithAppleProvider(): Promise<Session> {
  if (Platform.OS === 'android') {
    throw new AuthError(
      'Apple ile giriş yalnızca iOS’ta kullanılabilir. Lütfen Google ile devam edin.',
    );
  }

  // isSupported bir fonksiyon değil, doğrudan okunan bir özelliktir.
  if (!AppleAuthentication.isSupported) {
    throw new AuthError('Bu cihazda Apple ile giriş desteklenmiyor.');
  }

  // Apple nonce istiyor: cihazda üretip backend'e aynen gönderiyoruz.
  const rawNonce = Crypto.randomUUID();
  const hashedNonce = await Crypto.digestStringAsync(
    Crypto.CryptoDigestAlgorithm.SHA256,
    rawNonce,
  );

  try {
    const credential = await AppleAuthentication.performRequest({
      nonce: hashedNonce,
      requestedScopes: [
        AppleAuthentication.Scope.FULL_NAME,
        AppleAuthentication.Scope.EMAIL,
      ],
    });

    if (!credential.identityToken) {
      throw new AuthError('Apple kimlik belirteci alınamadı.');
    }

    const { fullName } = credential;

    return authApi.signIn('apple', {
      token: credential.identityToken,
      // Apple fullName'i yalnızca ilk yetkilendirmede döner; bu yüzden
      // backend'e istemciden iletmemiz gerekiyor.
      name: fullName ? `${fullName.givenName ?? ''} ${fullName.familyName ?? ''}`.trim() : undefined,
      nonce: rawNonce,
    });
  } catch (error) {
    if (error instanceof AuthError) throw error;
    if (isErrorWithCode(error) && error.code === statusCodes.IN_PROGRESS) {
      throw new AuthError('Apple girişi zaten sürüyor.');
    }
    if (isErrorWithCode(error) && error.code === statusCodes.SIGN_IN_CANCELLED) {
      throw new AuthError('Giriş iptal edildi.');
    }
    throw new AuthError('Apple ile giriş yapılamadı.');
  }
}

const decodeJwt = (token: string): Record<string, unknown> | null => {
  try {
    const [, payload] = token.split('.');
    if (!payload) return null;
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(normalized));
  } catch {
    return null;
  }
};

export const authService = {
  signInWithGoogle: signInWithGoogleProvider,
  signInWithApple: signInWithAppleProvider,

  /** Oturumu güvenli depodan geri yükler; süresi dolmuşsa null döner. */
  async restoreSession(): Promise<Session | null> {
    const raw = await SecureStore.getItemAsync(env.sessionStorageKey);
    if (!raw) return null;

    try {
      const session = JSON.parse(raw) as Session;

      const claims = decodeJwt(session.token);
      const exp = typeof claims?.exp === 'number' ? claims.exp : 0;
      // 60 saniye pay bırakılır: istek uçarken token süresi dolmasın.
      if (!exp || Date.now() >= exp * 1000 - 60_000) {
        await SecureStore.deleteItemAsync(env.sessionStorageKey);
        return null;
      }

      return session;
    } catch {
      await SecureStore.deleteItemAsync(env.sessionStorageKey);
      return null;
    }
  },

  async signOut(): Promise<void> {
    await SecureStore.deleteItemAsync(env.sessionStorageKey);

    // Google oturumu cihazda açık kalmasın.
    try {
      ensureGoogleConfigured();
      await GoogleSignin.signOut();
    } catch {
      // Google yapılandırılmadıysa sorun değil.
    }
  },

  /** Oturum token'ını başlıklara eklemek için dışa açar. */
  async currentToken(): Promise<string | null> {
    const session = await authService.restoreSession();
    return session?.token ?? null;
  },
};

export type { AuthUser };