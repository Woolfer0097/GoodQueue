import type { QueueAttemptState } from '@/entities/queue-attempt';

export type TerminalQueueAttemptState = Exclude<
  QueueAttemptState,
  'waiting' | 'invited' | 'checkout'
>;

interface ResultStatePresentation {
  actionLabel: string;
  actionTarget: 'catalog' | 'product' | 'retry';
  description: string;
  showRelevantProducts: boolean;
  title: string;
}

const resultStatePresentations: Record<TerminalQueueAttemptState, ResultStatePresentation> = {
  cancelled: {
    actionLabel: 'Вернуться к товару',
    actionTarget: 'product',
    description: 'Вы можете вернуться к товару и начать снова или перейти в каталог.',
    showRelevantProducts: true,
    title: 'Покупка отменена',
  },
  checkout_expired: {
    actionLabel: 'Повторить покупку',
    actionTarget: 'retry',
    description: 'Резерв закончился. Попробуйте купить товар ещё раз.',
    showRelevantProducts: true,
    title: 'Время оформления истекло',
  },
  invite_expired: {
    actionLabel: 'Попробовать снова',
    actionTarget: 'product',
    description:
      'Мы больше не можем держать товар за вами. Вернитесь к товару, чтобы попробовать снова.',
    showRelevantProducts: false,
    title: 'Время резерва истекло',
  },
  payment_failed: {
    actionLabel: 'Повторить покупку',
    actionTarget: 'retry',
    description: 'Попробуйте ещё раз или выберите другой товар.',
    showRelevantProducts: true,
    title: 'Оплата не прошла',
  },
  purchased: {
    actionLabel: 'Купить ещё',
    actionTarget: 'product',
    description: 'Если товар ещё в наличии, вы можете купить ещё один.',
    showRelevantProducts: false,
    title: 'Покупка завершена',
  },
  sold_out: {
    actionLabel: 'Вернуться в каталог',
    actionTarget: 'catalog',
    description: 'Выбранного товара больше нет в наличии. Посмотрите доступные альтернативы.',
    showRelevantProducts: true,
    title: 'Товар закончился',
  },
};

export const isTerminalQueueAttemptState = (
  state: QueueAttemptState,
): state is TerminalQueueAttemptState =>
  state !== 'waiting' && state !== 'invited' && state !== 'checkout';

export const getResultStatePresentation = (state: TerminalQueueAttemptState) =>
  resultStatePresentations[state];
