import AppleAuthentication from '@invertase/react-native-apple-authentication';
import {
  configure,
  getTokens,
  signIn as googleSignIn,
  signOut as googleSignOut,
} from '@react-native-google-signin/google-signin';
import * as Crypto from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';

import { ApiError, authApi } from '../api/client';
import type { AuthUser, Session } from '../api/types';
import { env } from '../config/env';

let configured = false;

export class AuthError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = 'AuthError';
  }
}

export const authService = {
  configure(): void {
    if (configured) {
      return;
    }

    configure({
      webClientId: env.googleWebClientId,
      iosClientId: env.googleIosClientId,
    });
    configured = true;
  },

  async signInWithGoogle(): Promise<Session> {
    this.configure();

    const { error } = await googleSignIn();
    if (error) {
      throw new AuthError('Google girişi tamamlanamadı', 'google_cancelled');
    }

    const { data, error: tokenError } = await getTokens();
    if (tokenError || !data?.idToken) {
      throw new AuthError('Google kimlik tokenı alınamadı', 'google_token_failed');
    }

    const session = await authApi.signIn('google', { token: data.idToken });
    await persistSession(session);
    return session;
  },

  async signInWithApple(): Promise<Session> {
    const available = await AppleAuthentication.isSupportedAsync();
    if (!available) {
      throw new AuthError('Bu cihazda Apple girişi kullanılamıyor', 'apple_unsupported');
    }

    const rawNonce = generateNonce();
    const hashedNonce = await sha256(rawNonce);

    const credential = await AppleAuthentication.signInAsync({
      nonce: hashedNonce,
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
    });

    if (!credential.identityToken) {
      throw new AuthError('Apple kimlik tokenı alınamadı', 'apple_token_failed');
    }

    const fullName = [credential.fullName?.givenName, credential.fullName?.familyName]
      .filter(Boolean)
      .join(' ');

    const session = await authApi.signIn('apple', {
      token: credential.identityToken,
      nonce: rawNonce,
      name: fullName.length > 0 ? fullName : undefined,
    });
    await persistSession(session);

    return session;
  },

  async restoreSession(): Promise<Session | null> {
    const raw = await SecureStore.getItemAsync(env.sessionStorageKey);
    if (!raw) {
      return null;
    }

    const session = parseSession(raw);
    if (!session || new Date(session.expiresAt).getTime() <= Date.now()) {
      await this.signOut();
      return null;
    }

    try {
      const user = await authApi.me(session.token);
      const restored: Session = { ...session, user };
      await persistSession(restored);
      return restored;
    } catch (error) {
      if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
        await this.signOut();
        return null;
      }
      throw error;
    }
  },

  async currentUser(): Promise<AuthUser | null> {
    const session = await this.restoreSession();
    return session?.user ?? null;
  },

  async signOut(): Promise<void> {
    await SecureStore.deleteItemAsync(env.sessionStorageKey);

    try {
      await googleSignOut();
    } catch {
      return;
    }
  },
};

async function persistSession(session: Session): Promise<void> {
  await SecureStore.setItemAsync(env.sessionStorageKey, JSON.stringify(session), {
    keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
  });
}

function parseSession(raw: string): Session | null {
  try {
    return JSON.parse(raw) as Session;
  } catch {
    return null;
  }
}

function generateNonce(length = 32): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  const bytes = new Uint8Array(length);
  Crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => alphabet[byte % alphabet.length]).join('');
}

function sha256(value: string): Promise<string> {
  return Crypto.digestStringAsync(Crypto.CryptoDigestAlgorithm.SHA256, value);
}