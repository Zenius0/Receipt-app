import { useCallback, useEffect, useState } from 'react';

import type { AuthUser, Session } from '../api/types';
import { AuthError, authService } from '../services/authService';

interface SessionState {
  user: AuthUser | null;
  session: Session | null;
  isLoading: boolean;
  error: string | null;
  /** Google native modülü mevcut mu? Expo Go'da false. */
  googleAvailable: boolean;
}

export function useSession(): SessionState & {
  signInWithGoogle: () => Promise<void>;
  signInWithApple: () => Promise<void>;
  signOut: () => Promise<void>;
} {
  const [state, setState] = useState<SessionState>({
    user: null,
    session: null,
    isLoading: true,
    error: null,
    googleAvailable: false,
  });

  const run = useCallback(async (action: () => Promise<Session | null>) => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const session = await action();
      // prev ile birleştiriliyor: googleAvailable gibi eşzamanlı çözülen
      // alanlar bu güncellemede silinmemeli.
      setState((prev) => ({
        ...prev,
        user: session?.user ?? null,
        session,
        isLoading: false,
        error: null,
      }));
    } catch (error) {
      const message =
        error instanceof AuthError || error instanceof Error ? error.message : 'Bilinmeyen hata';
      setState((prev) => ({ ...prev, user: null, session: null, isLoading: false, error: message }));
    }
  }, []);

  useEffect(() => {
    void run(() => authService.restoreSession());
  }, [run]);

  // Google native modülü hazır mı diye bak. Expo Go'da bu import reddedilir
  // ve false döner; uygulamanın açılmasını engellemez.
  useEffect(() => {
    let active = true;
    void authService.isGoogleAvailable().then((available) => {
      if (active) setState((prev) => ({ ...prev, googleAvailable: available }));
    });
    return () => {
      active = false;
    };
  }, []);

  return {
    ...state,
    signInWithGoogle: () => run(() => authService.signInWithGoogle()),
    signInWithApple: () => run(() => authService.signInWithApple()),
    signOut: () => run(() => authService.signOut().then(() => null)),
  };
}