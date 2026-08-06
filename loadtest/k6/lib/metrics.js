import { Counter, Rate, Trend } from 'k6/metrics';

export const joinDuration = new Trend('join_duration', true);
export const currentDuration = new Trend('current_duration', true);
export const joinSuccess = new Rate('join_success');
export const currentSuccess = new Rate('current_success');
export const duplicateJoinSuccess = new Rate('duplicate_join_success');
export const unexpectedRequestFailureRate = new Rate('unexpected_request_failure_rate');
export const unexpected4xx = new Counter('unexpected_4xx');
export const unexpected5xx = new Counter('unexpected_5xx');
export const joinErrors = new Counter('join_errors');
export const currentErrors = new Counter('current_errors');
export const stateWaiting = new Counter('state_waiting');
export const stateInvited = new Counter('state_invited');
export const stateCheckout = new Counter('state_checkout');
export const stateTerminal = new Counter('state_terminal');
export const pollingRequests = new Counter('polling_requests');

export function metricTags(operation, result, productGroup) {
  return { operation, result, product_group: productGroup };
}

export function recordState(entry, tags) {
  switch (entry.state) {
    case 'waiting':
      stateWaiting.add(1, tags);
      break;
    case 'invited':
      stateInvited.add(1, tags);
      break;
    case 'checkout':
      stateCheckout.add(1, tags);
      break;
    default:
      stateTerminal.add(1, tags);
      break;
  }
}

export function recordUnexpectedStatus(response, tags) {
  if (response.status >= 500) {
    unexpected5xx.add(1, tags);
  } else if (response.status >= 400) {
    unexpected4xx.add(1, tags);
  }
}
