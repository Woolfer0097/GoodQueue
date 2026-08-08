import { getProductAvailability, type ProductAvailability } from './product.availability';
import type { Product } from './product.schema';

export const PRODUCT_IMAGE_PLACEHOLDER =
  'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 800 600%22%3E%3Crect width=%22800%22 height=%22600%22 fill=%22%23f1f3f5%22/%3E%3Cpath d=%22M300 390l75-90 55 65 45-50 75 75H300z%22 fill=%22%23ced4da%22/%3E%3Ccircle cx=%22345%22 cy=%22245%22 r=%2232%22 fill=%22%23ced4da%22/%3E%3C/svg%3E';

const availabilityPresentation: Record<ProductAvailability, { color: string; label: string }> = {
  available: { color: 'green', label: 'В наличии' },
  available_by_queue: { color: 'yellow', label: 'Доступно по очереди' },
  queue_full: { color: 'orange', label: 'Очередь заполнена' },
  sold_out: { color: 'gray', label: 'Нет в наличии' },
  unavailable: { color: 'gray', label: 'Покупка временно недоступна' },
};

const priceFormatter = new Intl.NumberFormat('ru-RU', {
  currency: 'RUB',
  maximumFractionDigits: 2,
  minimumFractionDigits: 0,
  style: 'currency',
});

const productCategoryLabels: Record<string, string> = {
  audio: 'Аудио',
  collectibles: 'Коллекционирование',
  electronics: 'Электроника',
  music: 'Музыка',
  sneakers: 'Кроссовки',
  watches: 'Часы',
};

export const formatProductCategory = (category: string) =>
  productCategoryLabels[category] ?? category.replaceAll('_', ' ');

export const formatProductPrice = (priceCents: number) =>
  priceFormatter.format(priceCents / 100).replaceAll('\u00a0', ' ');

export const getProductAvailabilityPresentation = (product: Product) =>
  availabilityPresentation[getProductAvailability(product)];
