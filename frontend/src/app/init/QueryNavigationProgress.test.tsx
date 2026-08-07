import { MantineProvider } from '@mantine/core';
import { nprogressStore } from '@mantine/nprogress';
import { QueryClient, QueryClientProvider, useMutation, useQuery } from '@tanstack/react-query';
import { act, render, waitFor } from '@testing-library/react';
import { useEffect } from 'react';

import { QueryNavigationProgress } from './QueryNavigationProgress';

function createDeferred() {
  let resolve!: (value: string) => void;
  const promise = new Promise<string>((resolvePromise) => {
    resolve = resolvePromise;
  });

  return { promise, resolve };
}

function QueryActivity({
  background = false,
  request,
}: {
  background?: boolean;
  request: Promise<string>;
}) {
  useQuery({
    meta: background ? { background: true } : undefined,
    queryFn: () => request,
    queryKey: ['progress-test', background, request],
  });

  return null;
}

function MutationActivity({ request }: { request: Promise<string> }) {
  const { mutate } = useMutation({ mutationFn: () => request });

  useEffect(() => {
    mutate();
  }, [mutate]);

  return null;
}

describe('QueryNavigationProgress', () => {
  it('stays active until all queries and mutations complete', async () => {
    const query = createDeferred();
    const mutation = createDeferred();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <MantineProvider>
          <QueryNavigationProgress />
          <QueryActivity request={query.promise} />
          <MutationActivity request={mutation.promise} />
        </MantineProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(nprogressStore.getState().mounted).toBe(true);
    });

    await act(async () => {
      query.resolve('query complete');
      await query.promise;
    });

    expect(nprogressStore.getState().mounted).toBe(true);

    await act(async () => {
      mutation.resolve('mutation complete');
      await mutation.promise;
    });

    await waitFor(() => {
      expect(nprogressStore.getState().progress).toBe(100);
    });
  });

  it('ignores background queries', async () => {
    const query = createDeferred();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <MantineProvider>
          <QueryNavigationProgress />
          <QueryActivity background request={query.promise} />
        </MantineProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(queryClient.isFetching()).toBe(1);
    });
    expect(nprogressStore.getState().mounted).toBe(false);

    await act(async () => {
      query.resolve('query complete');
      await query.promise;
    });
  });
});
