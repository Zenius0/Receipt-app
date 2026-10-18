import React from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';

import { Banner, Screen } from '../components/Layout';
import { Button } from '../components/Button';
import { colors, radius, spacing, typography } from '../theme';

interface SignInScreenProps {
  isLoading: boolean;
  error: string | null;
  googleEnabled: boolean;
  onSignInWithGoogle: () => void;
  onSignInWithApple: () => void;
}

/**
 * Giriş ekranı.
 *
 * Tasarım kararı: tek bir siyah buton. Kullanıcı ekrana geldiğinde "Google
 * ile devam et" yazısı ve başka hiçbir şey görür — hangi sağlayıcının
 * yapılandırılmış olduğu belli olmadığı için her iki buton da gösterilir,
 * ancak birincil olan Google'dur (Apple üyeliği maliyetlidir ve çoğu
 * projede kapalıdır).
 */
export function SignInScreen({
  isLoading,
  error,
  googleEnabled,
  onSignInWithGoogle,
  onSignInWithApple,
}: SignInScreenProps) {
  return (
    <Screen>
      <View style={styles.container}>
        <View style={styles.brandBlock}>
          <View style={styles.mark}>
            <Text style={styles.markText}>₺</Text>
          </View>

          <Text style={styles.title}>Fatura Takipçi</Text>
          <Text style={styles.subtitle}>
            Fişin fotoğrafını çekin, tutarı ve kategorisini otomatik dolduralım.
          </Text>
        </View>

        {error ? (
          <Banner tone="error" title="Giriş yapılamadı" message={error} />
        ) : null}

        <View style={styles.actions}>
          {googleEnabled ? (
            <Button
              label="Google ile devam et"
              icon="G"
              onPress={onSignInWithGoogle}
              loading={isLoading}
              testID="sign-in-google"
            />
          ) : (
            <Banner
              tone="info"
              title="Google girişi yapılandırılmamış"
              message="EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID değerini .env dosyanıza ekleyin."
            />
          )}

          <Button
            label="Apple ile devam et"
            icon=""
            variant="secondary"
            size="medium"
            onPress={onSignInWithApple}
            disabled={isLoading}
            testID="sign-in-apple"
          />
        </View>

        {isLoading ? (
          <View style={styles.loadingRow}>
            <ActivityIndicator color={colors.textTertiary} />
          </View>
        ) : null}

        <Text style={styles.legal}>
          Devam ederek fiş görsellerinin okunması için işlenebileceğini kabul etmiş olursunuz.
          Görseller cihazınızda saklanır.
        </Text>
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: 'center',
  },
  brandBlock: {
    alignItems: 'center',
    marginBottom: spacing.xxxl,
  },
  mark: {
    width: 76,
    height: 76,
    borderRadius: radius.pill,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: spacing.lg,
  },
  markText: {
    color: colors.textOnDark,
    fontSize: 38,
    fontWeight: '700',
  },
  title: {
    ...typography.largeTitle,
    color: colors.text,
    letterSpacing: -0.8,
  },
  subtitle: {
    ...typography.body,
    color: colors.textSecondary,
    textAlign: 'center',
    marginTop: spacing.xs,
    maxWidth: 320,
    lineHeight: 23,
  },
  actions: {
    gap: spacing.sm,
  },
  loadingRow: {
    marginTop: spacing.lg,
    alignItems: 'center',
  },
  legal: {
    ...typography.footnote,
    color: colors.textTertiary,
    textAlign: 'center',
    marginTop: spacing.xl,
    lineHeight: 16,
  },
});