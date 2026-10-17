export type AuthProvider = 'google' | 'apple';

export interface AuthUser {
  id: string;
  email: string;
  name: string | null;
  provider: AuthProvider;
  accountantEmail: string | null;
  createdAt: string;
}

export interface Session {
  token: string;
  tokenType: string;
  expiresAt: string;
  user: AuthUser;
}

export interface SignInPayload {
  token: string;
  name?: string;
  nonce?: string;
}