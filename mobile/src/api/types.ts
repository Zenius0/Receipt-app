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

/** Backend'in kabul ettiği kategori kodları (receipts_category_chk ile aynı). */
export type ReceiptCategory =
  | 'fuel'
  | 'food'
  | 'groceries'
  | 'repair'
  | 'software'
  | 'office'
  | 'travel'
  | 'health'
  | 'entertainment'
  | 'clothing'
  | 'utilities'
  | 'other';

/** POST /receipts/scan yanıtı — kullanıcı onayından ÖNCE gelen öneri. */
export interface ScanDraft {
  storeName: string;
  totalAmount: number;
  currency: string;
  category: ReceiptCategory;
  receiptDate: string;
  taxNumber?: string;
}

/** POST /receipts/confirm gövdesi. */
export interface ConfirmReceiptPayload {
  storeName: string;
  totalAmount: number;
  currency: string;
  category: ReceiptCategory;
  receiptDate: string;
  localImagePath?: string | null;
}

/** Kaydedilmiş fiş satırı. */
export interface Receipt extends ConfirmReceiptPayload {
  id: string;
  userId: string;
  receiptDate: string;
  localImagePath: string | null;
  createdAt: string;
}

/** GET /receipts sayfalanmış yanıtı. */
export interface ReceiptPage {
  items: Receipt[];
  nextOffset?: number;
  total: number;
}

/** 429 quota_exceeded hatasının taşıdığı bilgiler. */
export interface QuotaInfo {
  limit: number;
  resetsAt: string;
}