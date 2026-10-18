import React, { type ReactNode } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View, type StyleProp, type ViewStyle } from 'react-native';
import { SafeAreaView } from 'react-native';

import { colors, radius, shadow, spacing, typography } from '../theme';

/** Ekran kabuğu: güvenli alan + beyaz zemin. */
export function Screen({
  children,
  scroll = false,
  footer,
  style,
}: {
  children: ReactNode;
  scroll?: boolean;
  /** Sabit alt alan (buton gibi) — kaydırılabilir içerikten bağımsız durur. */
  footer?: ReactNode;
  style?: StyleProp<ViewStyle>;
}) {
  const content = scroll ? (
    <ScrollView
      contentContainerStyle={styles.scrollContent}
      showsVerticalScrollIndicator={false}
      keyboardShouldPersistTaps="handled"
    >
      {children}
    </ScrollView>
  ) : (
    <View style={styles.staticContent}>{children}</View>
  );

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={[styles.body, style]}>{content}</View>
      {footer ? <View style={styles.footer}>{footer}</View> : null}
    </SafeAreaView>
  );
}

/** Yumuşak gölgeli kart. */
export function Card({
  children,
  style,
  onPress,
  accessibilityLabel,
}: {
  children: ReactNode;
  style?: StyleProp<ViewStyle>;
  onPress?: () => void;
  accessibilityLabel?: string;
}) {
  if (onPress) {
    return (
      <Pressable
        onPress={onPress}
        accessibilityRole="button"
        accessibilityLabel={accessibilityLabel}
        style={({ pressed }) => [styles.card, shadow.card, pressed && styles.cardPressed, style]}
      >
        {children}
      </Pressable>
    );
  }

  return <View style={[styles.card, shadow.card, style]}>{children}</View>;
}

/** Bölüm başlığı — büyük, kalın, bol boşluklu. */
export function SectionHeader({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <View style={styles.sectionHeader}>
      <Text style={styles.sectionTitle}>{title}</Text>
      {action}
    </View>
  );
}

/** Kullanıcının ne yapacağını anlamasını sağlayan boş durum. */
export function EmptyState({
  emoji,
  title,
  description,
  action,
}: {
  emoji: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <View style={styles.emptyState}>
      <Text style={styles.emptyEmoji}>{emoji}</Text>
      <Text style={styles.emptyTitle}>{title}</Text>
      <Text style={styles.emptyDescription}>{description}</Text>
      {action ? <View style={styles.emptyAction}>{action}</View> : null}
    </View>
  );
}

/** Bilgi / hata kutusu. */
export function Banner({
  tone,
  title,
  message,
}: {
  tone: 'info' | 'error' | 'success';
  title: string;
  message?: string;
}) {
  const palette = {
    info: { background: colors.infoSurface, color: colors.text },
    error: { background: colors.dangerSurface, color: colors.danger },
    success: { background: colors.successSurface, color: colors.success },
  }[tone];

  return (
    <View style={[styles.banner, { backgroundColor: palette.background }]}>
      <Text style={[styles.bannerTitle, { color: tone === 'info' ? colors.text : palette.color }]}>
        {title}
      </Text>
      {message ? <Text style={styles.bannerMessage}>{message}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: colors.background,
  },
  body: {
    flex: 1,
  },
  staticContent: {
    flex: 1,
    paddingHorizontal: spacing.lg,
  },
  scrollContent: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.huge,
  },
  footer: {
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    paddingBottom: spacing.md,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.border,
    backgroundColor: colors.background,
  },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    padding: spacing.md,
    borderCurve: 'continuous',
  },
  cardPressed: {
    backgroundColor: colors.surfaceMuted,
    transform: [{ scale: 0.995 }],
  },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: spacing.xxl,
    marginBottom: spacing.sm,
  },
  sectionTitle: {
    ...typography.title3,
    color: colors.text,
    letterSpacing: -0.4,
  },
  emptyState: {
    alignItems: 'center',
    paddingVertical: spacing.xxl,
    paddingHorizontal: spacing.lg,
  },
  emptyEmoji: {
    fontSize: 44,
    marginBottom: spacing.md,
  },
  emptyTitle: {
    ...typography.title3,
    color: colors.text,
    marginBottom: spacing.xs,
    textAlign: 'center',
  },
  emptyDescription: {
    ...typography.subhead,
    color: colors.textSecondary,
    textAlign: 'center',
    maxWidth: 300,
  },
  emptyAction: {
    marginTop: spacing.lg,
    alignSelf: 'stretch',
  },
  banner: {
    borderRadius: radius.md,
    padding: spacing.md,
    borderCurve: 'continuous',
  },
  bannerTitle: {
    ...typography.subhead,
    fontWeight: '600',
  },
  bannerMessage: {
    ...typography.footnote,
    color: colors.textSecondary,
    marginTop: spacing.xxs,
  },
});