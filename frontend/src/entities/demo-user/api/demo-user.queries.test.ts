import { demoUserQueryKeys } from './demo-user.query-keys';

describe('demo user query keys', () => {
  it('provides a stable key for the demo user list', () => {
    expect(demoUserQueryKeys.list()).toEqual(['demo-users', 'list']);
    expect(demoUserQueryKeys.list()).toEqual(demoUserQueryKeys.list());
  });
});
