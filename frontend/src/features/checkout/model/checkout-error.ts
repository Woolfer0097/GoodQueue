import { ApiError } from '@/shared/api';

export const getCheckoutErrorNotification = (error: unknown) => {
  if (error instanceof ApiError && error.status === 410) {
    return {
      message: 'Обновите состояние попытки и попробуйте начать покупку заново.',
      title: 'Время оформления истекло',
    };
  }

  if (error instanceof ApiError && error.status === 409) {
    return {
      message: 'Backend больше не разрешает это действие для текущей попытки.',
      title: 'Право на покупку недоступно',
    };
  }

  return {
    message: 'Проверьте соединение и попробуйте ещё раз.',
    title: 'Не удалось завершить покупку',
  };
};
