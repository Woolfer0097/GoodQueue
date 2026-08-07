import { jest } from '@jest/globals';
import { ZodError } from 'zod';

const apiClientMock = jest.fn<(path: string) => Promise<unknown>>();

jest.unstable_mockModule('@/shared/api', () => ({
  apiClient: apiClientMock,
}));

const { getDemoUsers } = await import('./demo-user.api');

const demoUserResponse = {
  display_name: 'Пользователь 1',
  external_user_id: '00000000-0000-4000-8000-000000000001',
};

describe('demo user API', () => {
  beforeEach(() => {
    apiClientMock.mockReset();
  });

  it('requests and validates demo users', async () => {
    apiClientMock.mockResolvedValue([demoUserResponse]);

    await expect(getDemoUsers()).resolves.toEqual([demoUserResponse]);
    expect(apiClientMock).toHaveBeenCalledWith('/api/v1/demo/users');
  });

  it('rejects an invalid response before it reaches consumers', async () => {
    apiClientMock.mockResolvedValue([{ ...demoUserResponse, external_user_id: 'user-1' }]);

    await expect(getDemoUsers()).rejects.toBeInstanceOf(ZodError);
  });
});
