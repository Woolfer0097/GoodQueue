import { useEffect, useRef, useState } from 'react';

const COUNTDOWN_TICK_MS = 1_000;

const getRemainingSeconds = (deadline: string) =>
  Math.max(0, Math.ceil((Date.parse(deadline) - Date.now()) / COUNTDOWN_TICK_MS));

export const formatCountdown = (remainingSeconds: number | null) => {
  if (remainingSeconds === null) {
    return '--:--';
  }

  const minutes = Math.floor(remainingSeconds / 60);
  const seconds = remainingSeconds % 60;

  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
};

export const useDeadlineCountdown = (deadline: string | undefined, onExpire: () => void) => {
  const [, renderNextTick] = useState(0);
  const expiredDeadlineRef = useRef<string | undefined>(undefined);
  const onExpireRef = useRef(onExpire);

  useEffect(() => {
    onExpireRef.current = onExpire;
  }, [onExpire]);

  useEffect(() => {
    if (deadline === undefined) {
      return;
    }

    const requestCurrentAttempt = () => {
      renderNextTick((tick) => tick + 1);

      if (expiredDeadlineRef.current !== deadline) {
        expiredDeadlineRef.current = deadline;
        onExpireRef.current();
      }
    };
    const remainingMilliseconds = Date.parse(deadline) - Date.now();

    if (remainingMilliseconds <= 0) {
      requestCurrentAttempt();
      return;
    }

    const intervalId = window.setInterval(() => {
      renderNextTick((tick) => tick + 1);
    }, COUNTDOWN_TICK_MS);
    const expirationId = window.setTimeout(() => {
      window.clearInterval(intervalId);
      requestCurrentAttempt();
    }, remainingMilliseconds);

    return () => {
      window.clearInterval(intervalId);
      window.clearTimeout(expirationId);
    };
  }, [deadline]);

  return deadline === undefined ? null : getRemainingSeconds(deadline);
};
