import * as Crypto from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

import { ApiError, authApi } from '../api/client';
import type { AuthUser, Session } from '../api/types';
import { env } from '../config/env';

type GoogleSigninModule = typeof import('@react-native-google-signin/google-signin');
type AppleAuthModule = (typeof import('@invertase/react-native-apple-authentication'))['default'];

/**
 * Hem @react-native-google-signin/google-signin hem de
 * @invertase/react-native-apple-authentication native modüllerdir ve Expo Go
 * paketinde bulunmayabilir. Bu yüzden ikisi de dosya yüklenirken değil, ilk
 * ihtiyaç duyulduğunda getiriliyor: üstte statik import yapsaydık modül
 * bulunamadığında uygulama açılışta çökerdi.
 */
const GOOGLE_UNAVAILABLE =
  'Google girişi bu ortamda kullanılamıyor. Expo Go içinde çalışmaz; denemek için uygulamayı geliştirme derlemesi (development build) olarak kurmanız gerekir.';

const APPLE_UNAVAILABLE =
  'Apple ile giriş bu ortamda kullanılamıyor. Expo Go içinde çalışmaz; denemek için uygulamayı geliştirme derlemesi (development build) olarak kurmanız gerekir.';

let googleModule: GoogleSigninModule | null = null;
let googleModulePromise: Promise<GoogleSigninModule | null> | null = null;

function loadGoogleSignin(): Promise<GoogleSigninModule | null> {
  if (!googleModulePromise) {
    googleModulePromise = import('@react-native-google-signin/google-signin')
      .then((mod) => {
        googleModule = mod;
        return mod;
      })
      .catch(() => null);
  }
  return googleModulePromise;
}

/** Google modülünü getirir; yoksa kullanıcıya anlaşılır mesajla hata verir. */
async function requireGoogleSignin(): Promise<GoogleSigninModule> {
  const mod = await loadGoogleSignin();
  if (!mod) throw new AuthError(GOOGLE_UNAVAILABLE);
  return mod;
}

let appleModule: AppleAuthModule | null = null;
let appleModulePromise: Promise<AppleAuthModule | null> | null = null;

function loadAppleAuth(): Promise<AppleAuthModule | null> {
  if (!appleModulePromise) {
    appleModulePromise = import('@invertase/react-native-apple-authentication')
      .then((mod) => {
        appleModule = mod.default;
        return mod.default;
      })
      .catch(() => null);
  }
  return appleModulePromise;
}

/** Apple modülünü getirir; yoksa kullanıcıya anlaşılır mesajla hata verir. */
async function requireAppleAuth(): Promise<AppleAuthModule> {
  const mod = await loadAppleAuth();
  if (!mod) throw new AuthError(APPLE_UNAVAILABLE);
  return mod;
}

/** Google hata kodlarını güvenle okur; modül yüklü değilse null döner. */
function googleErrorCode(error: unknown): string | null {
  if (!googleModule || !googleModule.isErrorWithCode(error)) return null;
  return error.code;
}

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
function ensureGoogleConfigured(mod: GoogleSigninModule): void {
  if (configured) return;

  mod.GoogleSignin.configure({
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

  const mod = await requireGoogleSignin();
  const { GoogleSignin } = mod;

  ensureGoogleConfigured(mod);

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

  const AppleAuthentication = await requireAppleAuth();

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
    const codes = googleModule?.statusCodes;
    const code = googleErrorCode(error);
    if (codes && code === codes.IN_PROGRESS) {
      throw new AuthError('Apple girişi zaten sürüyor.');
    }
    if (codes && code === codes.SIGN_IN_CANCELLED) {
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

  /**
   * Google girişinin bu ortamda kullanılabilir olup olmadığını söyler.
   * Expo Go'da false döner; geliştirme derlemesinde true.
   */
  isGoogleAvailable: async (): Promise<boolean> => (await loadGoogleSignin()) !== null,

  /** Apple girişinin bu ortamda kullanılabilir olup olmadığını söyler. */
  isAppleAvailable: async (): Promise<boolean> => (await loadAppleAuth()) !== null,

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

    // Google oturumu cihazda açık kalmasın. Modül hiç yüklenmediyse
    // (örneğin Expo Go) yapılacak bir şey yok.
    try {
      await googleModule?.GoogleSignin.signOut();
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