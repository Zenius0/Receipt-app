import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';
import * as ImagePicker from 'expo-image-picker';

import { ApiError, receiptsApi, type ReceiptListQuery } from '../api/client';
import type { ConfirmReceiptPayload, Receipt, ReceiptPage, ScanDraft } from '../api/types';
import { env } from '../config/env';

export type PickSource = 'camera' | 'library';

export interface PickedImage {
  uri: string;
  base64: string;
  mimeType: string;
}

export class ScanCancelled extends Error {
  constructor() {
    super('Kullanıcı fiş seçimini iptal etti');
    this.name = 'ScanCancelled';
  }
}

export class ImageTooLarge extends Error {
  constructor(readonly sizeMb: number) {
    super('Görsel çok büyük');
    this.name = 'ImageTooLarge';
  }
}

/** Backend'in kabul ettiği azami gövde boyutu (12 MB). */
const MAX_UPLOAD_BYTES = 8 * 1024 * 1024;

const requestCamera = async (): Promise<boolean> => {
  const permission = await ImagePicker.requestCameraPermissionsAsync();
  return permission.granted;
};

const requestLibrary = async (): Promise<boolean> => {
  const permission = await ImagePicker.requestMediaLibraryPermissionsAsync();
  return permission.granted;
};

/**
 * Kullanıcıdan fiş görseli alır, AI'ya uygun şekilde küçültür ve base64'e
 * çevirir.
 *
 * Küçültme neden önemli: iPhone fotoğrafı tipik olarak 3-5 MB'dir; base64'e
 * çevrildiğinde ~4-7 MB olur ve yapay zekâya çok daha pahalı gider. 1600 px'e
 * indirmek yükü ~200 KB'a indirir ve okunabilirliği bozmaz.
 */
export async function pickReceiptImage(source: PickSource): Promise<PickedImage> {
  const granted =
    source === 'camera' ? await requestCamera() : await requestLibrary();
  if (!granted) {
    throw new Error(
      source === 'camera'
        ? 'Kamera erişimi gerekli. Ayarlar > Gizlilik > Kamera üzerinden açabilirsiniz.'
        : 'Fotoğraf erişimi gerekli. Ayarlar > Gizlilik > Fotoğraflar üzerinden açabilirsiniz.',
    );
  }

  const result = await ImagePicker.launchCameraAsync({
    mediaTypes: ['images'],
    quality: 1,
    allowsEditing: false,
    exif: false,
  });

  if (result.canceled) throw new ScanCancelled();

  const asset = result.assets[0];
  if (!asset?.uri) throw new Error('Görsel alınamadı.');

  return prepareImage(asset.uri);
}

/** Görseli küçültür ve base64 üretir. */
export async function prepareImage(uri: string): Promise<PickedImage> {
  // expo-image-manipulator'ın güncel (context tabanlı) API'si.
  // manipulateAsync sürüm 58'de deprecated işaretli.
  const context = ImageManipulator.manipulate(uri);
  context.resize({ width: env.maxScanImageWidth });

  const rendered = await context.renderAsync();
  const saved = await rendered.saveAsync({
    compress: 0.72,
    format: SaveFormat.JPEG,
  });

  // base64 alınmadan önce bayt boyutundan kontrol: 12 MB sınırını aşan
  // yüklemeler sunucuya gönderilmeden elenir.
  const base64 = await readAsBase64(saved.uri);
  const approxBytes = (base64.length * 3) / 4;
  if (approxBytes > MAX_UPLOAD_BYTES) {
    throw new ImageTooLarge(approxBytes / (1024 * 1024));
  }

  return { uri: saved.uri, base64, mimeType: 'image/jpeg' };
}

/**
 * Yerel dosyayı base64'e çevirir.
 * fetch + blob + FileReader; React Native'da Buffer/Node fs yoktur.
 */
async function readAsBase64(uri: string): Promise<string> {
  const response = await fetch(uri);
  const blob = await response.blob();

  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error('Görsel okunamadı.'));
    reader.onloadend = () => {
      const result = reader.result;
      if (typeof result !== 'string') {
        reject(new Error('Görsel okunamadı.'));
        return;
      }
      // data:image/jpeg;base64,XXXX → yalnızca base64 kısmı
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.readAsDataURL(blob);
  });
}

export const receiptService = {
  /** Fiş görselini yapay zekâya gönderir ve onay önerisini döner. */
  async scan(token: string, image: PickedImage): Promise<ScanDraft> {
    try {
      return await receiptsApi.scan(token, image.base64, image.mimeType);
    } catch (error) {
      throw toFriendlyError(error);
    }
  },

  confirm(token: string, payload: ConfirmReceiptPayload): Promise<Receipt> {
    return receiptsApi.confirm(token, payload);
  },

  list(token: string, query: ReceiptListQuery = {}): Promise<ReceiptPage> {
    return receiptsApi.list(token, query);
  },
};

/**
 * Backend hata kodlarını kullanıcının ne yapacağını anlayacağı metne çevirir.
 * Ham HTTP mesajları göstermek yerine ne yapması gerektiğini söylüyoruz.
 */
function toFriendlyError(error: unknown): Error {
  if (!(error instanceof ApiError)) {
    return error instanceof Error ? error : new Error('Beklenmeyen bir hata oluştu.');
  }

  if (error.isQuotaExceeded) {
    return new Error(
      'Günlük tarama hakkınız doldu. Yarın tekrar deneyebilirsiniz.',
    );
  }
  if (error.isNotAReceipt) {
    // Backend modelin açıklamasını taşır; anlaşılır olduğu için korunur.
    return new Error(error.message || 'Bu görselde okunabilir bir fiş bulunamadı.');
  }
  if (error.code === 'incomplete_receipt') {
    return new Error(
      `Fiş okundu ancak bazı bilgiler eksik: ${error.message}. Alanları elle düzeltebilirsiniz.`,
    );
  }
  if (error.code === 'unsupported_image') {
    return new Error('Bu görsel formatı desteklenmiyor. JPEG, PNG, WEBP veya HEIC kullanın.');
  }
  if (error.code === 'image_too_large') {
    return new Error('Görsel çok büyük. Farklı bir fotoğraf deneyin.');
  }
  if (error.code === 'timeout' || error.status === 504) {
    return new Error('Fiş okunması uzun sürdü. Tekrar deneyin.');
  }

  return new Error(error.message);
}