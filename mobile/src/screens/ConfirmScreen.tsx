import React, { useMemo, useState } from 'react';
import { Image, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';

import type { ReceiptCategory } from '../api/types';
import { Button } from '../components/Button';
import { Banner, Card } from '../components/Layout';
import {
  categoryMeta,
  colors,
  formatMoney,
  radius,
  spacing,
  typography,
} from '../theme';

export interface ConfirmDraft {
  storeName: string;
  totalAmount: string;
  currency: string;
  category: ReceiptCategory;
  receiptDate: string;
}

interface ConfirmScreenProps {
  draft: ConfirmDraft;
  imageUri: string;
  isSaving: boolean;
  error: string | null;
  onSave: (draft: ConfirmDraft) => void;
  onRetake: () => void;
  onCancel: () => void;
}

const currencies = ['TRY', 'USD', 'EUR', 'GBP'];

/**
 * Onay ekranı.
 *
 * Yapay zekâ çıkarımı HİÇBİR ZAMAN doğrudan kaydedilmez. Kullanıcı burada
 * her alanı görebilir ve gerekirse düzeltebilir. Bu, hem yanlış okumaların
 * veritabanına girmesini engeller hem de kullanıcıya kontrol hissi verir.
 */
export function ConfirmScreen({
  draft,
  imageUri,
  isSaving,
  error,
  onSave,
  onRetake,
  onCancel,
}: ConfirmScreenProps) {
  const [storeName, setStoreName] = useState(draft.storeName);
  const [amount, setAmount] = useState(draft.totalAmount);
  const [currency, setCurrency] = useState(draft.currency);
  const [category, setCategory] = useState<ReceiptCategory>(draft.category);
  const [receiptDate, setReceiptDate] = useState(draft.receiptDate);

  const parsedAmount = Number(amount.replace(',', '.'));
  const amountValid = Number.isFinite(parsedAmount) && parsedAmount >= 0;
  const canSave = storeName.trim().length > 0 && amountValid;

  const categories = useMemo(
    () => (Object.keys(categoryMeta) as ReceiptCategory[]).map((key) => ({ key, ...categoryMeta[key] })),
    [],
  );

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.content}
        showsVerticalScrollIndicator={false}
        keyboardShouldPersistTaps="handled"
      >
        <View style={styles.header}>
          <Text style={styles.title}>Kontrol edin</Text>
          <Text style={styles.subtitle}>
            Okunan bilgileri düzenleyebilirsiniz. Kaydetmek için aşağı kaydırın.
          </Text>
        </View>

        <Card style={styles.imageCard}>
          <Image source={{ uri: imageUri }} style={styles.image} resizeMode="cover" />
        </Card>

        {error ? <Banner tone="error" title="Kaydedilemedi" message={error} /> : null}

        <Text style={styles.label}>İşletme</Text>
        <TextInput
          value={storeName}
          onChangeText={setStoreName}
          placeholder="İşletme adı"
          placeholderTextColor={colors.textTertiary}
          style={styles.input}
          testID="input-store"
        />

        <Text style={styles.label}>Tutar</Text>
        <View style={styles.amountRow}>
          <TextInput
            value={amount}
            onChangeText={setAmount}
            keyboardType="decimal-pad"
            placeholder="0.00"
            placeholderTextColor={colors.textTertiary}
            style={[styles.input, styles.amountInput]}
            testID="input-amount"
          />
          <View style={styles.currencyRow}>
            {currencies.map((code) => (
              <Pressable
                key={code}
                onPress={() => setCurrency(code)}
                accessibilityRole="button"
                accessibilityState={{ selected: currency === code }}
                style={[styles.currencyChip, currency === code && styles.currencyChipActive]}
              >
                <Text style={[styles.currencyText, currency === code && styles.currencyTextActive]}>
                  {code}
                </Text>
              </Pressable>
            ))}
          </View>
        </View>
        {!amountValid && amount.length > 0 ? (
          <Text style={styles.errorText}>Geçerli bir tutar girin.</Text>
        ) : null}

        <Text style={styles.label}>Kategori</Text>
        <View style={styles.categoryGrid}>
          {categories.map((item) => {
            const active = item.key === category;
            return (
              <Pressable
                key={item.key}
                onPress={() => setCategory(item.key)}
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
                style={[styles.categoryChip, active && styles.categoryChipActive]}
              >
                <Text style={styles.categoryEmoji}>{item.emoji}</Text>
                <Text style={[styles.categoryLabel, active && styles.categoryLabelActive]}>
                  {item.label}
                </Text>
              </Pressable>
            );
          })}
        </View>

        <Text style={styles.label}>Tarih</Text>
        <TextInput
          value={receiptDate}
          onChangeText={setReceiptDate}
          placeholder="YYYY-MM-DD"
          placeholderTextColor={colors.textTertiary}
          style={styles.input}
          autoCapitalize="none"
          testID="input-date"
        />
      </ScrollView>

      {/* Sabit alt alan: eylem her zaman görünür, kaydırmaya gerek yok */}
      <View style={styles.footer}>
        {amountValid ? (
          <Text style={styles.summaryText}>
            {formatMoney(parsedAmount, currency)} · {categoryMeta[category]?.label}
          </Text>
        ) : null}

        <Button
          label={isSaving ? 'Kaydediliyor…' : 'Kaydet'}
          onPress={() => onSave({ storeName, totalAmount: amount, currency, category, receiptDate })}
          disabled={!canSave}
          loading={isSaving}
          testID="confirm-save"
        />

        <View style={styles.footerRow}>
          <Button label="Vazgeç" variant="ghost" size="medium" onPress={onCancel} style={styles.footerButton} />
          <Button
            label="Yeni fotoğraf"
            variant="secondary"
            size="medium"
            onPress={onRetake}
            style={styles.footerButton}
          />
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  content: {
    padding: spacing.lg,
    paddingBottom: spacing.xxl,
  },
  header: {
    paddingTop: spacing.md,
    marginBottom: spacing.md,
  },
  title: {
    ...typography.largeTitle,
    color: colors.text,
    letterSpacing: -0.8,
  },
  subtitle: {
    ...typography.subhead,
    color: colors.textSecondary,
    marginTop: spacing.xxs,
  },
  imageCard: {
    padding: spacing.xs,
    marginBottom: spacing.lg,
    shadowOpacity: 0.08,
  },
  image: {
    width: '100%',
    height: 170,
    borderRadius: radius.md,
    backgroundColor: colors.surfaceMuted,
  },
  label: {
    ...typography.footnote,
    color: colors.textSecondary,
    fontWeight: '600',
    marginBottom: spacing.xs,
    marginTop: spacing.md,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  input: {
    backgroundColor: colors.surfaceMuted,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    minHeight: 48,
    ...typography.body,
    color: colors.text,
  },
  amountRow: {
    gap: spacing.sm,
  },
  amountInput: {
    fontSize: 22,
    fontWeight: '700',
    letterSpacing: -0.5,
  },
  currencyRow: {
    flexDirection: 'row',
    gap: spacing.xs,
  },
  currencyChip: {
    flex: 1,
    paddingVertical: spacing.sm,
    borderRadius: radius.sm,
    backgroundColor: colors.surfaceMuted,
    alignItems: 'center',
  },
  currencyChipActive: {
    backgroundColor: colors.primary,
  },
  currencyText: {
    ...typography.subhead,
    color: colors.textSecondary,
  },
  currencyTextActive: {
    color: colors.textOnDark,
    fontWeight: '700',
  },
  errorText: {
    ...typography.footnote,
    color: colors.danger,
    marginTop: spacing.xxs,
  },
  categoryGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.xs,
  },
  categoryChip: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: spacing.sm,
    paddingVertical: spacing.xs,
    borderRadius: radius.pill,
    backgroundColor: colors.surfaceMuted,
    gap: spacing.xxs,
  },
  categoryChipActive: {
    backgroundColor: colors.primary,
  },
  categoryEmoji: {
    fontSize: 14,
  },
  categoryLabel: {
    ...typography.footnote,
    color: colors.textSecondary,
  },
  categoryLabelActive: {
    color: colors.textOnDark,
    fontWeight: '600',
  },
  footer: {
    padding: spacing.lg,
    paddingBottom: spacing.xl,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.border,
    backgroundColor: colors.background,
    gap: spacing.sm,
  },
  summaryText: {
    ...typography.subhead,
    color: colors.textSecondary,
    textAlign: 'center',
  },
  footerRow: {
    flexDirection: 'row',
    gap: spacing.sm,
  },
  footerButton: {
    flex: 1,
  },
});