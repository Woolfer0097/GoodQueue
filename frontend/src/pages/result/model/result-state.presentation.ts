import type { QueueAttemptState } from '@/entities/queue-attempt';

export type TerminalQueueAttemptState = Exclude<
  QueueAttemptState,
  'waiting' | 'invited' | 'checkout'
>;

interface ResultStatePresentation {
  actionLabel: string;
  actionTarget: 'catalog' | 'product' | 'retry';
  description: string;
  title: string;
}

const resultStatePresentations: Record<TerminalQueueAttemptState, ResultStatePresentation> = {
  cancelled: {
    actionLabel: 'Вернуться к товару',
    actionTarget: 'product',
    description:
      'Попытка завершена. Вы можете вернуться к товару и при необходимости начать снова.',
    title: 'Вы вышли из очереди',
  },
  checkout_expired: {
    actionLabel: 'Повторить покупку',
    actionTarget: 'retry',
    description: 'Отведённое на оформление время закончилось. Начните покупку заново.',
    title: 'Время оформления истекло',
  },
  invite_expired: {
    actionLabel: 'Попробовать снова',
    actionTarget: 'product',
    description:
      'Срок персонального резерва закончился. Вернитесь к товару, чтобы попробовать снова.',
    title: 'Время резерва истекло',
  },
  payment_failed: {
    actionLabel: 'Повторить покупку',
    actionTarget: 'retry',
    description: 'Покупка не завершена. Вы можете попробовать снова или выбрать другой товар.',
    title: 'Не удалось завершить покупку',
  },
  purchased: {
    actionLabel: 'Вернуться в каталог',
    actionTarget: 'catalog',
    description: 'Покупка успешно подтверждена. Товар закреплён за вами.',
    title: 'Покупка подтверждена',
  },
  sold_out: {
    actionLabel: 'Вернуться в каталог',
    actionTarget: 'catalog',
    description: 'Выбранного товара больше нет в наличии. Посмотрите доступные альтернативы.',
    title: 'Товар закончился',
  },
};

export const isTerminalQueueAttemptState = (
  state: QueueAttemptState,
): state is TerminalQueueAttemptState =>
  state !== 'waiting' && state !== 'invited' && state !== 'checkout';

export const getResultStatePresentation = (state: TerminalQueueAttemptState) =>
  resultStatePresentations[state];
