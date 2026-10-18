import React from 'react';
import { Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import type { AuthUser, Receipt } from '../api/types';
import { Button } from '../components/Button';
import { Card, EmptyState, SectionHeader, Screen } from '../components/Layout';
import { ReceiptRow } from '../components/ReceiptRow';
import { categoryMeta, colors, formatMoney, spacing, typography } from '../theme';

interface HomeScreenProps {
  user: AuthUser;
  receipts: Receipt[];
  totalCount: number;
  isRefreshing: boolean;
  isScanning: boolean;
  onScan: () => void;
  onRefresh: () => void;
  onSeeAll: () => void;
  onSignOut: () => void;
}

/**
 * Ana ekran.
 *
 * Burada tek bir kural var: kullanıcı açtığı anda ne yapması gerektiğini
 * görmeli. Bu yüzden ekranın üstünde tek büyük siyah "Fiş Tara" kartı var;
 * altında yalnızca "son hareketler" bilgisi yer alıyor. Ayarlar, istatistik,
 * grafik gibi ekranlar bilinçli olarak yok — tek eylem, tek ekran.
 */
export function HomeScreen({
  user,
  receipts,
  totalCount,
  isRefreshing,
  isScanning,
  onScan,
  onRefresh,
  onSeeAll,
  onSignOut,
}: HomeScreenProps) {
  const firstName = user.name?.split(' ')[0] ?? user.email.split('@')[0] ?? '';
  const recent = receipts.slice(0, 5);
  const monthTotal = receipts
    .filter((r) => {
      const date = new Date(r.receiptDate);
      const now = new Date();
      return date.getMonth() === now.getMonth() && date.getFullYear() === now.getFullYear();
    })
    .reduce((sum, r) => sum + r.totalAmount, 0);

  return (
    <Screen
      scroll
      footer={
        <Button label="Çıkış yap" variant="ghost" size="medium" onPress={onSignOut} />
      }
    >
      <View style={styles.header}>
        <View style={styles.headerText}>
          <Text style={styles.greeting}>Merhaba</Text>
          <Text style={styles.name} numberOfLines={1}>
            {firstName}
          </Text>
        </View>
      </View>

      {/* ANA EYLEM — ekranın tek siyah butonu */}
      <Pressable
        onPress={onScan}
        disabled={isScanning}
        accessibilityRole="button"
        accessibilityLabel="Fiş tara"
        testID="scan-button"
        style={({ pressed }) => [styles.scanCard, pressed && styles.scanCardPressed, isScanning && styles.scanCardDisabled]}
      >
        <View style={styles.scanIconCircle}>
          <Text style={styles.scanIcon}>📷</Text>
        </View>

        <View style={styles.scanCopy}>
          <Text style={styles.scanTitle}>{isScanning ? 'Fiş okunuyor…' : 'Fiş Tara'}</Text>
          <Text style={styles.scanSubtitle}>
            {isScanning
              ? 'Yapay zekâ fişinizi analiz ediyor, biraz sürer'
              : 'Fişin fotoğrafını çekin veya galeriden seçin'}
          </Text>
        </View>

        <Text style={styles.chevron}>›</Text>
      </Pressable>

      {/* Kısa özet — kullanıcıya nerede olduğunu söyler */}
      <View style={styles.summaryRow}>
        <View style={styles.summaryItem}>
          <Text style={styles.summaryValue}>{totalCount}</Text>
          <Text style={styles.summaryLabel}>Toplam fiş</Text>
        </View>
        <View style={styles.summaryDivider} />
        <View style={styles.summaryItem}>
          <Text style={styles.summaryValue}>
            {monthTotal > 0 ? formatMoney(monthTotal, 'TRY') : '—'}
          </Text>
          <Text style={styles.summaryLabel}>Bu ay</Text>
        </View>
      </View>

      <SectionHeader
        title="Son fişler"
        action={
          receipts.length > 0 ? (
            <Pressable onPress={onSeeAll} accessibilityRole="button">
              <Text style={styles.seeAll}>Tümü</Text>
            </Pressable>
          ) : null
        }
      />

      {recent.length === 0 ? (
        <EmptyState
          emoji="🧾"
          title="Henüz fiş yok"
          description="Yukarıdaki düğmeye dokunup ilk fişinizi ekleyin. Görsel cihazınızda kalır."
        />
      ) : (
        recent.map((receipt) => (
          <ReceiptRow key={receipt.id} receipt={receipt} />
        ))
      )}

      {receipts.length > 0 ? (
        <Card style={styles.privacyNote}>
          <Text style={styles.privacyText}>
            📱 Görseller cihazınızda saklanır. Sunucuya yalnızca tutar ve kategori gibi
            metinsel bilgiler gönderilir.
          </Text>
        </Card>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  header: {
    paddingTop: spacing.lg,
    paddingBottom: spacing.lg,
  },
  headerText: {
    gap: 2,
  },
  greeting: {
    ...typography.subhead,
    color: colors.textSecondary,
  },
  name: {
    ...typography.largeTitle,
    color: colors.text,
    letterSpacing: -0.8,
  },
  scanCard: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.primary,
    borderRadius: 20,
    padding: spacing.lg,
    gap: spacing.md,
  },
  scanCardPressed: {
    backgroundColor: colors.primaryPressed,
    transform: [{ scale: 0.99 }],
  },
  scanCardDisabled: {
    opacity: 0.7,
  },
  scanIconCircle: {
    width: 52,
    height: 52,
    borderRadius: 26,
    backgroundColor: 'rgba(255,255,255,0.15)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  scanIcon: {
    fontSize: 26,
  },
  scanCopy: {
    flex: 1,
  },
  scanTitle: {
    ...typography.title3,
    color: colors.textOnDark,
    letterSpacing: -0.4,
  },
  scanSubtitle: {
    ...typography.footnote,
    color: 'rgba(255,255,255,0.75)',
    marginTop: 2,
    lineHeight: 17,
  },
  chevron: {
    fontSize: 30,
    color: 'rgba(255,255,255,0.5)',
    fontWeight: '300',
    marginTop: -4,
  },
  summaryRow: {
    flexDirection: 'row',
    alignItems: 'center',
    marginTop: spacing.md,
    backgroundColor: colors.surfaceMuted,
    borderRadius: 16,
    paddingVertical: spacing.md,
  },
  summaryItem: {
    flex: 1,
    alignItems: 'center',
  },
  summaryValue: {
    ...typography.title2,
    color: colors.text,
    letterSpacing: -0.5,
  },
  summaryLabel: {
    ...typography.caption,
    color: colors.textSecondary,
    marginTop: 2,
  },
  summaryDivider: {
    width: StyleSheet.hairlineWidth,
    height: 32,
    backgroundColor: colors.borderStrong,
  },
  seeAll: {
    ...typography.subhead,
    color: colors.text,
    fontWeight: '600',
  },
  privacyNote: {
    marginTop: spacing.lg,
    backgroundColor: colors.surfaceMuted,
    shadowOpacity: 0,
    elevation: 0,
  },
  privacyText: {
    ...typography.footnote,
    color: colors.textSecondary,
    lineHeight: 17,
  },
});