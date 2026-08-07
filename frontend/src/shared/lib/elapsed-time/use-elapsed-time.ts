import { useEffect, useState } from 'react';

const ELAPSED_TIME_TICK_MS = 1_000;

export const formatElapsedTime = (elapsedSeconds: number) => {
  const hours = Math.floor(elapsedSeconds / 3_600);
  const minutes = Math.floor((elapsedSeconds % 3_600) / 60);
  const seconds = elapsedSeconds % 60;
  const timeParts = [minutes, seconds].map((part) => String(part).padStart(2, '0'));

  if (hours > 0) {
    timeParts.unshift(String(hours).padStart(2, '0'));
  }

  return timeParts.join(':');
};

export const useElapsedTime = (startedAt: string | undefined) => {
  const [currentTime, setCurrentTime] = useState(Date.now);

  useEffect(() => {
    if (startedAt === undefined) {
      return;
    }

    const intervalId = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, ELAPSED_TIME_TICK_MS);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [startedAt]);

  if (startedAt === undefined) {
    return null;
  }

  return Math.max(0, Math.floor((currentTime - Date.parse(startedAt)) / ELAPSED_TIME_TICK_MS));
};
