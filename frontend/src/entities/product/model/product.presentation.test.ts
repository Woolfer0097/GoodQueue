import { formatProductCategory } from './product.presentation';

describe('product presentation', () => {
  it.each([
    ['collectibles', 'Коллекционирование'],
    ['sneakers', 'Кроссовки'],
    ['watches', 'Часы'],
    ['music', 'Музыка'],
    ['electronics', 'Электроника'],
  ])('formats the backend category "%s" for users', (category, label) => {
    expect(formatProductCategory(category)).toBe(label);
  });

  it('keeps an unknown backend category readable', () => {
    expect(formatProductCategory('limited_items')).toBe('limited items');
  });
});
