import { useCallback, useEffect, useState } from 'react';

import type { AuthUser, Session } from '../api/types';
import { AuthError, authService } from '../services/authService';

interface SessionState {
  user: AuthUser | null;
  session: Session | null;
  isLoading: boolean;
  error: string | null;
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
  });

  const run = useCallback(async (action: () => Promise<Session | null>) => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const session = await action();
      setState({ user: session?.user ?? null, session, isLoading: false, error: null });
    } catch (error) {
      const message =
        error instanceof AuthError || error instanceof Error ? error.message : 'Bilinmeyen hata';
      setState({ user: null, session: null, isLoading: false, error: message });
    }
  }, []);

  useEffect(() => {
    void run(() => authService.restoreSession());
  }, [run]);

  return {
    ...state,
    signInWithGoogle: () => run(() => authService.signInWithGoogle()),
    signInWithApple: () => run(() => authService.signInWithApple()),
    signOut: () => run(() => authService.signOut().then(() => null)),
  };
}