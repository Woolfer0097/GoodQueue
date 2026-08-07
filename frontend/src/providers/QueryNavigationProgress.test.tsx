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

function QueryActivity({ request }: { request: Promise<string> }) {
  useQuery({ queryFn: () => request, queryKey: ['progress-test', request] });

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
});
