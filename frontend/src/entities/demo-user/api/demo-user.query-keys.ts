const demoUserRootQueryKey = ['demo-users'] as const;

export const demoUserQueryKeys = {
  all: demoUserRootQueryKey,
  list: () => [...demoUserRootQueryKey, 'list'] as const,
};
