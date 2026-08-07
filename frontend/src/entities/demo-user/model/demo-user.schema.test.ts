import { demoUserListSchema, demoUserSchema } from './demo-user.schema';

const demoUserResponse = {
  display_name: 'Пользователь 1',
  external_user_id: '00000000-0000-4000-8000-000000000001',
};

describe('demo user schemas', () => {
  it('parses a demo user from the backend contract', () => {
    expect(demoUserSchema.parse(demoUserResponse)).toEqual(demoUserResponse);
  });

  it('parses a list of demo users', () => {
    expect(demoUserListSchema.parse([demoUserResponse])).toEqual([demoUserResponse]);
  });

  it.each([
    ['invalid UUID', { ...demoUserResponse, external_user_id: 'user-1' }],
    ['empty display name', { ...demoUserResponse, display_name: '   ' }],
    ['missing display name', { external_user_id: demoUserResponse.external_user_id }],
  ])('rejects %s', (_caseName, response) => {
    expect(() => demoUserSchema.parse(response)).toThrow();
  });
});
