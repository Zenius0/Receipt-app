/**
 * Tasarım sistemi.
 *
 * Yön: beyaz zemin, siyah birincil buton, çok açık gri ayrım çizgileri.
 * Apple'ın arayüz dilinden esinlenir — az renk, çok boşluk, güçlü tipografi
 * hiyerarşisi ve dokunma hedeflerinin cömert olması.
 *
 * Tek kural: kullanıcı ekrana baktığında ne yapması gerektiğini bir bakışta
 * anlamalı. Bu yüzden her ekranda TEK bir siyah birincil eylem vardır.
 */

export const colors = {
  // Zeminler
  background: '#FFFFFF',
  surface: '#FFFFFF',
  surfaceMuted: '#F5F5F7', // kart arka planı, ikincil yüzeyler

  // Metin
  text: '#000000',
  textSecondary: '#6E6E73', // açıklamalar, yardımcı metin
  textTertiary: '#8E8E93',
  textOnDark: '#FFFFFF',

  // Çizgiler
  border: '#E5E5EA',
  borderStrong: '#D1D1D6',

  // Eylem
  primary: '#000000', // siyah buton
  primaryPressed: '#1C1C1E',
  disabled: '#C7C7CC',

  // Anlamsal
  danger: '#FF3B30',
  success: '#34C759',
  warning: '#FF9500',

  // Uyarı zemini
  dangerSurface: '#FFF2F1',
  successSurface: '#EFFAF3',
  infoSurface: '#F0F7FF',
} as const;

/** 4pt tabanlı boşluk ölçeği. */
export const spacing = {
  xxs: 4,
  xs: 8,
  sm: 12,
  md: 16,
  lg: 20,
  xl: 24,
  xxl: 32,
  xxxl: 40,
  huge: 56,
} as const;

export const radius = {
  sm: 10,
  md: 14,
  lg: 18,
  xl: 24,
  pill: 999,
} as const;

/**
 * Tipografi. iOS'ta sistem fontu (SF Pro) otomatik kullanılır; Android'de
 * Roboto'ya düşer. Satır yükseklikleri ayrıca verilir ki metin Apple gibi
 * nefes alsın.
 */
export const typography = {
  largeTitle: { fontSize: 34, lineHeight: 41, fontWeight: '700' },
  title1: { fontSize: 28, lineHeight: 34, fontWeight: '700' },
  title2: { fontSize: 22, lineHeight: 28, fontWeight: '700' },
  title3: { fontSize: 20, lineHeight: 25, fontWeight: '600' },
  headline: { fontSize: 17, lineHeight: 22, fontWeight: '600' },
  body: { fontSize: 17, lineHeight: 22, fontWeight: '400' },
  callout: { fontSize: 16, lineHeight: 21, fontWeight: '400' },
  subhead: { fontSize: 15, lineHeight: 20, fontWeight: '500' },
  footnote: { fontSize: 13, lineHeight: 18, fontWeight: '400' },
  caption: { fontSize: 12, lineHeight: 16, fontWeight: '500' },
} as const;

/** Platforma göre gölge (iOS daha belirgin, Android daha yumuşak). */
export const shadow = {
  card: {
    shadowColor: '#000',
    shadowOpacity: 0.05,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 2 },
    elevation: 2,
  },
  button: {
    shadowColor: '#000',
    shadowOpacity: 0.12,
    shadowRadius: 8,
    shadowOffset: { width: 0, height: 3 },
    elevation: 3,
  },
} as const;

/** Kategori kodları → Türkçe etiket ve ikon. */
export const categoryMeta: Record<string, { label: string; emoji: string }> = {
  fuel: { label: 'Yakıt', emoji: '⛽️' },
  food: { label: 'Yemek', emoji: '🍽️' },
  groceries: { label: 'Alışveriş', emoji: '🛒' },
  repair: { label: 'Tamir', emoji: '🔧' },
  software: { label: 'Yazılım', emoji: '💻' },
  office: { label: 'Kırtasiye', emoji: '📎' },
  travel: { label: 'Seyahat', emoji: '✈️' },
  health: { label: 'Sağlık', emoji: '⚕️' },
  entertainment: { label: 'Eğlence', emoji: '🎬' },
  clothing: { label: 'Giyim', emoji: '👕' },
  utilities: { label: 'Faturalar', emoji: '💡' },
  other: { label: 'Diğer', emoji: '📄' },
};

/** Para birimi sembolü. Bilinmeyen kodlar olduğu gibi gösterilir. */
export function formatMoney(amount: number, currency: string): string {
  const symbol = currencySymbols[currency] ?? currency;
  const formatted = amount.toLocaleString('tr-TR', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return `${symbol}${formatted}`;
}

export const currencySymbols: Record<string, string> = {
  TRY: '₺',
  USD: '$',
  EUR: '€',
  GBP: '£',
};

/** Tarihi Türkçe kısa biçimde gösterir: "17 Eki 2026". */
export function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleDateString('tr-TR', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  });
}

/** "3 dk önce" biçiminde göreli zaman. */
export function formatRelative(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';

  const diffMs = Date.now() - date.getTime();
  const minutes = Math.floor(diffMs / 60000);

  if (minutes < 1) return 'az önce';
  if (minutes < 60) return `${minutes} dk önce`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} saat önce`;

  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} gün önce`;

  return formatDate(iso);
}