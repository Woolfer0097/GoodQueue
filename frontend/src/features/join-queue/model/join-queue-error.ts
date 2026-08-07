import { ApiError } from '@/shared/api';

interface ErrorResponseData {
  error?: {
    code?: unknown;
  };
}

const getErrorCode = (error: ApiError) => {
  const data = error.data as ErrorResponseData | null;

  return typeof data?.error?.code === 'string' ? data.error.code : null;
};

export const shouldReuseJoinQueueIdempotencyKey = (error: unknown) =>
  !(error instanceof ApiError) || error.status >= 500;

export const getJoinQueueErrorNotification = (error: unknown) => {
  if (error instanceof ApiError && getErrorCode(error) === 'queue_disabled') {
    return {
      message: 'Для этого товара очередь сейчас отключена.',
      title: 'Покупка временно недоступна',
    };
  }

  if (error instanceof ApiError && error.status === 410) {
    return {
      message: 'Вернитесь в каталог и выберите другой товар.',
      title: 'Товар закончился',
    };
  }

  return {
    message: 'Проверьте соединение и попробуйте ещё раз.',
    title: 'Не удалось войти в очередь',
  };
};
