import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import type { Receipt } from '../api/types';
import { categoryMeta, colors, formatDate, formatMoney, radius, spacing, typography } from '../theme';
import { Card } from './Layout';

/** Liste satırı: kategori ikonu, işletme adı, tarih ve tutar. */
export function ReceiptRow({ receipt, onPress }: { receipt: Receipt; onPress?: () => void }) {
  const meta = categoryMeta[receipt.category] ?? categoryMeta.other!;

  return (
    <Card
      style={styles.card}
      onPress={onPress}
      accessibilityLabel={`${receipt.storeName}, ${formatMoney(receipt.totalAmount, receipt.currency)}`}
    >
      <View style={styles.row}>
        <View style={styles.iconCircle}>
          <Text style={styles.icon}>{meta.emoji}</Text>
        </View>

        <View style={styles.middle}>
          <Text style={styles.storeName} numberOfLines={1}>
            {receipt.storeName}
          </Text>
          <Text style={styles.metaLine} numberOfLines={1}>
            {meta.label} · {formatDate(receipt.receiptDate)}
          </Text>
        </View>

        <View style={styles.right}>
          <Text style={styles.amount}>{formatMoney(receipt.totalAmount, receipt.currency)}</Text>
        </View>
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  card: {
    marginBottom: spacing.xs,
    padding: spacing.sm,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  iconCircle: {
    width: 42,
    height: 42,
    borderRadius: radius.pill,
    backgroundColor: colors.surfaceMuted,
    alignItems: 'center',
    justifyContent: 'center',
    marginRight: spacing.sm,
  },
  icon: {
    fontSize: 20,
  },
  middle: {
    flex: 1,
    paddingRight: spacing.xs,
  },
  storeName: {
    ...typography.headline,
    color: colors.text,
    letterSpacing: -0.3,
  },
  metaLine: {
    ...typography.footnote,
    color: colors.textSecondary,
    marginTop: 2,
  },
  right: {
    alignItems: 'flex-end',
  },
  amount: {
    ...typography.headline,
    color: colors.text,
    letterSpacing: -0.3,
  },
});