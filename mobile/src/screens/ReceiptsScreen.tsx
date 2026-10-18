import React, { useMemo, useState } from 'react';
import { Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native';

import type { Receipt, ReceiptCategory } from '../api/types';
import { Button } from '../components/Button';
import { EmptyState } from '../components/Layout';
import { ReceiptRow } from '../components/ReceiptRow';
import { categoryMeta, colors, formatMoney, spacing, typography } from '../theme';

interface ReceiptsScreenProps {
  receipts: Receipt[];
  isRefreshing: boolean;
  onRefresh: () => void;
  onScan: () => void;
  onBack: () => void;
}

/** Tüm fişler — kategori filtresi ve toplam tutar. */
export function ReceiptsScreen({
  receipts,
  isRefreshing,
  onRefresh,
  onScan,
  onBack,
}: ReceiptsScreenProps) {
  const [filter, setFilter] = useState<ReceiptCategory | 'all'>('all');

  const filtered = useMemo(
    () => (filter === 'all' ? receipts : receipts.filter((r) => r.category === filter)),
    [receipts, filter],
  );

  const total = filtered.reduce((sum, r) => sum + r.totalAmount, 0);

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.header}>
        <Pressable onPress={onBack} accessibilityRole="button" accessibilityLabel="Geri" hitSlop={12}>
          <Text style={styles.back}>‹ Geri</Text>
        </Pressable>
        <Text style={styles.title}>Fişlerim</Text>
        <View style={styles.headerSpacer} />
      </View>

      <View style={styles.totalBar}>
        <Text style={styles.totalLabel}>
          {filtered.length} fiş{filter !== 'all' ? ` · ${categoryMeta[filter]?.label}` : ''}
        </Text>
        <Text style={styles.totalValue}>{formatMoney(total, 'TRY')}</Text>
      </View>

      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={styles.filterRow}
        style={styles.filterScroll}
      >
        <FilterChip label="Tümü" active={filter === 'all'} onPress={() => setFilter('all')} />
        {(Object.keys(categoryMeta) as ReceiptCategory[])
          .filter((key) => receipts.some((r) => r.category === key))
          .map((key) => (
            <FilterChip
              key={key}
              label={`${categoryMeta[key]?.emoji} ${categoryMeta[key]?.label}`}
              active={filter === key}
              onPress={() => setFilter(key)}
            />
          ))}
      </ScrollView>

      <ScrollView
        contentContainerStyle={styles.list}
        showsVerticalScrollIndicator={false}
        refreshControl={<RefreshControl refreshing={isRefreshing} onRefresh={onRefresh} />}
      >
        {filtered.length === 0 ? (
          <EmptyState
            emoji={receipts.length === 0 ? '🧾' : '🔍'}
            title={receipts.length === 0 ? 'Henüz fiş yok' : 'Bu kategoride fiş yok'}
            description={
              receipts.length === 0
                ? 'Fiş ekleyerek gider takibinizi başlatın.'
                : 'Başka bir kategori seçmeyi deneyin.'
            }
            action={
              receipts.length === 0 ? (
                <Button label="Fiş Tara" onPress={onScan} />
              ) : null
            }
          />
        ) : (
          filtered.map((receipt) => <ReceiptRow key={receipt.id} receipt={receipt} />)
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

function FilterChip({
  label,
  active,
  onPress,
}: {
  label: string;
  active: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
      style={[styles.chip, active && styles.chipActive]}
    >
      <Text style={[styles.chipText, active && styles.chipTextActive]}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: colors.background,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    paddingBottom: spacing.xs,
  },
  back: {
    ...typography.body,
    color: colors.text,
    width: 72,
  },
  title: {
    ...typography.title3,
    color: colors.text,
    letterSpacing: -0.4,
  },
  headerSpacer: {
    width: 72,
  },
  totalBar: {
    flexDirection: 'row',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
  },
  totalLabel: {
    ...typography.footnote,
    color: colors.textSecondary,
  },
  totalValue: {
    ...typography.title2,
    color: colors.text,
    letterSpacing: -0.5,
  },
  filterScroll: {
    flexGrow: 0,
  },
  filterRow: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
    gap: spacing.xs,
  },
  chip: {
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs,
    borderRadius: 999,
    backgroundColor: colors.surfaceMuted,
  },
  chipActive: {
    backgroundColor: colors.primary,
  },
  chipText: {
    ...typography.footnote,
    color: colors.textSecondary,
    fontWeight: '500',
  },
  chipTextActive: {
    color: colors.textOnDark,
    fontWeight: '600',
  },
  list: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.huge,
  },
});