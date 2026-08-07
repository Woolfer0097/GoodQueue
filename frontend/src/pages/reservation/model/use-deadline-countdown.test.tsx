import { jest } from '@jest/globals';
import { act, renderHook } from '@testing-library/react';

import { formatCountdown, useDeadlineCountdown } from './use-deadline-countdown';

describe('reservation deadline countdown', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-08-07T10:00:00Z'));
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('formats the remaining server deadline without storing a decrementing source of truth', () => {
    const onExpire = jest.fn();
    const { result } = renderHook(() => useDeadlineCountdown('2026-08-07T10:01:05Z', onExpire));

    expect(formatCountdown(result.current)).toBe('01:05');

    act(() => {
      jest.setSystemTime(new Date('2026-08-07T10:00:05Z'));
      jest.advanceTimersByTime(1_000);
    });

    expect(formatCountdown(result.current)).toBe('00:59');
    expect(onExpire).not.toHaveBeenCalled();
  });

  it('recalculates from a replacement deadline after a background refetch', () => {
    const onExpire = jest.fn();
    const { rerender, result } = renderHook(
      ({ deadline }) => useDeadlineCountdown(deadline, onExpire),
      { initialProps: { deadline: '2026-08-07T10:00:10Z' } },
    );

    expect(formatCountdown(result.current)).toBe('00:10');

    rerender({ deadline: '2026-08-07T10:02:00Z' });

    expect(formatCountdown(result.current)).toBe('02:00');
  });

  it('requests current backend state once when the countdown reaches zero', () => {
    const onExpire = jest.fn();
    const { result } = renderHook(() => useDeadlineCountdown('2026-08-07T10:00:01Z', onExpire));

    act(() => {
      jest.advanceTimersByTime(1_000);
    });

    expect(formatCountdown(result.current)).toBe('00:00');
    expect(onExpire).toHaveBeenCalledTimes(1);

    act(() => {
      jest.advanceTimersByTime(5_000);
    });

    expect(onExpire).toHaveBeenCalledTimes(1);
  });

  it('requests current backend state for a deadline that is already past', () => {
    const onExpire = jest.fn();

    renderHook(() => useDeadlineCountdown('2026-08-07T09:59:59Z', onExpire));

    expect(onExpire).toHaveBeenCalledTimes(1);
  });
});
