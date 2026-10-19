import React, { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, BackHandler, Platform, StyleSheet, Text, View } from 'react-native';
import { StatusBar } from 'expo-status-bar';

import { ConfirmScreen, type ConfirmDraft } from './src/screens/ConfirmScreen';
import { HomeScreen } from './src/screens/HomeScreen';
import { ReceiptsScreen } from './src/screens/ReceiptsScreen';
import { SignInScreen } from './src/screens/SignInScreen';
import { Button } from './src/components/Button';
import { env } from './src/config/env';
import { useSession } from './src/hooks/useSession';
import {
  ImageTooLarge,
  ScanCancelled,
  pickReceiptImage,
  receiptService,
  type PickedImage,
} from './src/services/receiptService';
import type { Receipt } from './src/api/types';
import { colors, spacing, typography } from './src/theme';

type Route = 'home' | 'confirm' | 'receipts';

export default function App() {
  const {
    user,
    session,
    isLoading,
    error: authError,
    googleAvailable,
    appleAvailable,
    signInWithGoogle,
    signInWithApple,
    signOut,
  } = useSession();

  const [route, setRoute] = useState<Route>('home');
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const [image, setImage] = useState<PickedImage | null>(null);
  const [draft, setDraft] = useState<ConfirmDraft | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const token = session?.token ?? null;

  // Apple ile giriş yalnızca iOS'ta çalışır (sistem özelliği). Android'de
  // butonu hiç göstermiyoruz — çalışmayan bir buton koymak yerine tek
  // seçenek bırakmak daha temiz.
  const showApple = Platform.OS === 'ios';

  // Expo Go'da native giriş modülleri yoktur; kullanıcıya bunu baştan söyle.
  const nativeAuthMissing = !googleAvailable || (showApple && !appleAvailable);

  // Fiş listesini çek. Oturum yoksa hiç denenmez.
  const loadReceipts = useCallback(async () => {
    if (!token) return;
    setIsRefreshing(true);
    try {
      const page = await receiptService.list(token, { limit: 50 });
      setReceipts(page.items);
    } catch (err) {
      // Oturum düşmüş olabilir; sessizce geç, kullanıcıyı rahatsız etme.
      if (err instanceof Error && 'status' in err && (err as { status?: number }).status === 401) {
        setReceipts([]);
      }
    } finally {
      setIsRefreshing(false);
    }
  }, [token]);

  useEffect(() => {
    void loadReceipts();
  }, [loadReceipts]);

  // Android'de geri tuşu ana ekrana dönmeli.
  useEffect(() => {
    const subscription = BackHandler.addEventListener('hardwareBackPress', () => {
      if (route !== 'home') {
        setRoute('home');
        return true;
      }
      return false;
    });
    return () => subscription.remove();
  }, [route]);

  /** Kamera/galeriden görsel al, AI'ya gönder, onay ekranını aç. */
  const handleScan = useCallback(async () => {
    if (!token || isScanning) return;

    setScanError(null);
    setSaveError(null);

    try {
      const picked = await pickReceiptImage('camera');

      setImage(picked);
      setIsScanning(true);

      const result = await receiptService.scan(token, picked);

      setDraft({
        storeName: result.storeName,
        totalAmount: String(result.totalAmount),
        currency: result.currency,
        category: result.category,
        // Onay ekranı "YYYY-MM-DD" bekler; backend RFC3339 döner.
        receiptDate: result.receiptDate.slice(0, 10),
      });
      setRoute('confirm');
    } catch (err) {
      if (err instanceof ScanCancelled) {
        // Kullanıcı vazgeçti — hata değil, sessizce çık.
      } else if (err instanceof ImageTooLarge) {
        setScanError(`Görsel çok büyük (${err.sizeMb.toFixed(1)} MB). Lütfen daha küçük bir fotoğraf çekin.`);
      } else {
        setScanError(err instanceof Error ? err.message : 'Fiş okunamadı.');
      }
    } finally {
      setIsScanning(false);
    }
  }, [token, isScanning]);

  const handleSave = useCallback(
    async (values: ConfirmDraft) => {
      if (!token || !image) return;

      setSaveError(null);
      setIsSaving(true);

      try {
        await receiptService.confirm(token, {
          storeName: values.storeName.trim(),
          totalAmount: Number(values.totalAmount.replace(',', '.')),
          currency: values.currency,
          category: values.category,
          // Onay ekranındaki "YYYY-MM-DD" gün ortasına sabitlenir; bu, saat
          // dilimi kaymasıyla bir gün kaymasını engeller.
          receiptDate: `${values.receiptDate}T12:00:00Z`,
          localImagePath: image.uri,
        });

        setRoute('home');
        setImage(null);
        setDraft(null);
        await loadReceipts();
      } catch (err) {
        setSaveError(err instanceof Error ? err.message : 'Fiş kaydedilemedi.');
      } finally {
        setIsSaving(false);
      }
    },
    [token, image, loadReceipts],
  );

  const handleCancel = useCallback(() => {
    setRoute('home');
    setImage(null);
    setDraft(null);
    setSaveError(null);
  }, []);

  if (isLoading) {
    return <SplashScreen />;
  }

  if (!user || !session) {
    return (
      <SignInScreen
        isLoading={isLoading}
        error={authError}
        googleConfigured={Boolean(env.googleWebClientId || env.googleIosClientId)}
        showApple={showApple}
        nativeAuthMissing={nativeAuthMissing}
        onSignInWithGoogle={() => void signInWithGoogle()}
        onSignInWithApple={() => void signInWithApple()}
      />
    );
  }

  // Tarama sırasında tam ekran kaplama — kullanıcı ne olduğunu bilsin.
  if (isScanning) {
    return <ScanningOverlay />;
  }

  if (route === 'confirm' && draft && image) {
    return (
      <ConfirmScreen
        draft={draft}
        imageUri={image.uri}
        isSaving={isSaving}
        error={saveError}
        onSave={(values) => void handleSave(values)}
        onRetake={() => {
          handleCancel();
          void handleScan();
        }}
        onCancel={handleCancel}
      />
    );
  }

  if (route === 'receipts') {
    return (
      <ReceiptsScreen
        receipts={receipts}
        isRefreshing={isRefreshing}
        onRefresh={() => void loadReceipts()}
        onScan={() => void handleScan()}
        onBack={() => setRoute('home')}
      />
    );
  }

  return (
    <View style={styles.root}>
      <HomeScreen
        user={user}
        receipts={receipts}
        totalCount={receipts.length}
        isRefreshing={isRefreshing}
        isScanning={isScanning}
        onScan={() => void handleScan()}
        onRefresh={() => void loadReceipts()}
        onSeeAll={() => setRoute('receipts')}
        onSignOut={() => void signOut()}
      />

      {scanError ? (
        <View style={styles.toastWrap}>
          <View style={styles.toast}>
            <Text style={styles.toastText}>{scanError}</Text>
            <Button
              label="Kapat"
              variant="ghost"
              size="medium"
              onPress={() => setScanError(null)}
              style={styles.toastButton}
            />
          </View>
        </View>
      ) : null}
    </View>
  );
}

/** Oturum geri yüklenirken gösterilen sade bir ekran. */
function SplashScreen() {
  return (
    <View style={styles.splash}>
      <View style={styles.splashMark}>
        <Text style={styles.splashMarkText}>₺</Text>
      </View>
      <ActivityIndicator color={colors.textTertiary} style={styles.splashSpinner} />
    </View>
  );
}

/** AI'ın fişi okuduğu sıre — süre öngörülemeyeceği için dürüst mesaj veriyoruz. */
function ScanningOverlay() {
  return (
    <View style={styles.overlay}>
      <View style={styles.overlayCard}>
        <ActivityIndicator size="large" color="#FFFFFF" />
        <Text style={styles.overlayTitle}>Fişiniz okunuyor</Text>
        <Text style={styles.overlaySubtitle}>
          Yapay zekâ işletme adını, tutarı ve kategoriyi çıkarıyor.{'\n'}Genellikle
          10-20 saniye sürer.
        </Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: colors.background,
  },
  splash: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.background,
  },
  splashMark: {
    width: 76,
    height: 76,
    borderRadius: 38,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  splashMarkText: {
    color: colors.textOnDark,
    fontSize: 38,
    fontWeight: '700',
  },
  splashSpinner: {
    marginTop: spacing.xl,
  },
  overlay: {
    ...StyleSheet.absoluteFill,
    backgroundColor: 'rgba(0,0,0,0.72)',
    alignItems: 'center',
    justifyContent: 'center',
    padding: spacing.xl,
  },
  overlayCard: {
    alignItems: 'center',
    gap: spacing.sm,
  },
  overlayTitle: {
    ...typography.title3,
    color: colors.textOnDark,
    marginTop: spacing.sm,
  },
  overlaySubtitle: {
    ...typography.footnote,
    color: 'rgba(255,255,255,0.75)',
    textAlign: 'center',
    lineHeight: 18,
  },
  toastWrap: {
    ...StyleSheet.absoluteFill,
    justifyContent: 'flex-end',
    padding: spacing.lg,
  },
  toast: {
    backgroundColor: colors.text,
    borderRadius: 14,
    padding: spacing.md,
    paddingLeft: spacing.md,
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
  },
  toastText: {
    ...typography.footnote,
    color: colors.textOnDark,
    flex: 1,
    lineHeight: 17,
  },
  toastButton: {
    paddingHorizontal: spacing.sm,
    minHeight: 32,
  },
});