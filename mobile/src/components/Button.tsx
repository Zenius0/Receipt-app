import React from 'react';
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
  type StyleProp,
  type ViewStyle,
} from 'react-native';

import { colors, radius, spacing, typography } from '../theme';

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
type Size = 'large' | 'medium';

interface ButtonProps {
  label: string;
  onPress: () => void;
  variant?: Variant;
  size?: Size;
  disabled?: boolean;
  loading?: boolean;
  /** Butonun yanında gösterilecek kısa sembol (emoji veya ikon). */
  icon?: string;
  style?: StyleProp<ViewStyle>;
  testID?: string;
}

/**
 * Tek birincil eylem siyahtır (Apple stili). Diğer her şey gri veya çıplak —
 * bu sayede kullanıcının gözü her ekranda ne yapacağını bulur.
 */
export function Button({
  label,
  onPress,
  variant = 'primary',
  size = 'large',
  disabled = false,
  loading = false,
  icon,
  style,
  testID,
}: ButtonProps) {
  const isDisabled = disabled || loading;

  return (
    <Pressable
      onPress={onPress}
      disabled={isDisabled}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled: isDisabled, busy: loading }}
      testID={testID}
      style={({ pressed }) => [
        styles.base,
        size === 'large' ? styles.large : styles.medium,
        variantStyles[variant].container,
        pressed && !isDisabled && variantStyles[variant].pressed,
        isDisabled && styles.disabled,
        style,
      ]}
    >
      <View style={styles.content}>
        {loading ? (
          <ActivityIndicator
            color={variant === 'primary' || variant === 'danger' ? colors.textOnDark : colors.text}
            style={styles.spinner}
          />
        ) : null}
        {icon && !loading ? <Text style={styles.icon}>{icon}</Text> : null}
        <Text style={[styles.label, size === 'large' ? styles.labelLarge : styles.labelMedium, variantStyles[variant].label]}>
          {label}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    borderRadius: radius.md,
    alignItems: 'center',
    justifyContent: 'center',
    borderCurve: 'continuous',
  },
  large: {
    minHeight: 54, // Apple'ın önerdiği minimum dokunma hedefi (44pt) üzerinde
    paddingHorizontal: spacing.xl,
  },
  medium: {
    minHeight: 44,
    paddingHorizontal: spacing.md,
  },
  content: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
  },
  label: {
    textAlign: 'center',
  },
  labelLarge: {
    fontSize: 17,
    lineHeight: 22,
    fontWeight: '600',
    letterSpacing: -0.2,
  },
  labelMedium: {
    fontSize: 15,
    lineHeight: 20,
    fontWeight: '600',
  },
  icon: {
    fontSize: 18,
    marginRight: spacing.xs,
  },
  spinner: {
    marginRight: spacing.xs,
  },
  disabled: {
    opacity: 0.45,
  },
});

const variantStyles: Record<Variant, { container: ViewStyle; pressed: ViewStyle; label: { color: string } }> = {
  primary: {
    container: { backgroundColor: colors.primary },
    pressed: { backgroundColor: colors.primaryPressed, transform: [{ scale: 0.985 }] },
    label: { color: colors.textOnDark },
  },
  secondary: {
    container: { backgroundColor: colors.surfaceMuted },
    pressed: { backgroundColor: colors.border },
    label: { color: colors.text },
  },
  ghost: {
    container: { backgroundColor: 'transparent' },
    pressed: { backgroundColor: colors.surfaceMuted },
    label: { color: colors.text },
  },
  danger: {
    container: { backgroundColor: colors.danger },
    pressed: { opacity: 0.85 },
    label: { color: colors.textOnDark },
  },
};