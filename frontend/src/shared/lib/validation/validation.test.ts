import { nonEmptyStringSchema, uuidSchema } from './validation';

describe('validation schemas', () => {
  describe('nonEmptyStringSchema', () => {
    it('trims a non-empty string', () => {
      expect(nonEmptyStringSchema.parse('  value  ')).toBe('value');
    });

    it('rejects a blank string', () => {
      expect(nonEmptyStringSchema.safeParse('   ').success).toBe(false);
    });
  });

  describe('uuidSchema', () => {
    it('accepts a UUID', () => {
      expect(uuidSchema.safeParse('00000000-0000-4000-8000-000000000001').success).toBe(true);
    });

    it('rejects a non-UUID string', () => {
      expect(uuidSchema.safeParse('not-a-uuid').success).toBe(false);
    });
  });
});
