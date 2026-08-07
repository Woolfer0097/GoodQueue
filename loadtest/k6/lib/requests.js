import http from 'k6/http';

import {
  currentDuration,
  currentErrors,
  currentSuccess,
  actionErrors,
  cancelDuration,
  cancelSuccess,
  checkoutDuration,
  checkoutSuccess,
  duplicateJoinSuccess,
  joinDuration,
  joinErrors,
  joinSuccess,
  metricTags,
  paymentDuration,
  paymentSuccess,
  pollingRequests,
  recordState,
  recordUnexpectedStatus,
  unexpectedRequestFailureRate,
} from './metrics.js';

const joinName = 'POST /api/v1/products/:productID/queue-entries';
const currentName = 'GET /api/v1/products/:productID/queue-entry';
const checkoutName = 'POST /api/v1/queue-attempts/:attemptID/checkout';
const cancelName = 'DELETE /api/v1/products/:productID/queue-entry';
const paymentName = 'POST /internal/v1/payment-events';
const queueStates = new Set([
  'waiting', 'invited', 'checkout', 'purchased', 'invite_expired',
  'checkout_expired', 'payment_failed', 'cancelled', 'sold_out',
]);
const expectedJoinErrors = new Set(['queue_full', 'queue_disabled', 'already_purchased', 'sold_out']);

export function join(config, user, assignment) {
  const response = http.post(
    `${config.baseURL}/api/v1/products/${assignment.product_id}/queue-entries`,
    null,
    {
      headers: { 'X-User-ID': user.id, 'Idempotency-Key': assignment.idempotency_key },
      tags: { name: joinName, operation: 'join', product_group: assignment.product_group },
      responseCallback: http.expectedStatuses(200, 201, 409, 410),
    },
  );
  const classified = classifyJoin(response);
  const tags = metricTags('join', classified.result, assignment.product_group);
  joinDuration.add(response.timings.duration, tags);
  joinSuccess.add(classified.success, tags);
  unexpectedRequestFailureRate.add(classified.result === 'unexpected_error', tags);
  if (classified.result === 'unexpected_error') {
    joinErrors.add(1, tags);
    recordUnexpectedStatus(response, tags);
  }
  if (classified.entry !== null) {
    recordState(classified.entry, tags);
  }
  return classified;
}

export function duplicateJoin(config, user, assignment, original) {
  const replay = join(config, user, assignment);
  const successful = replay.success && replay.response.status === 200 && replay.entry.attempt_id === original.attempt_id;
  const result = successful ? 'success' : 'unexpected_error';
  const tags = metricTags('join', result, assignment.product_group);
  duplicateJoinSuccess.add(successful, tags);
  if (!successful) {
    unexpectedRequestFailureRate.add(true, tags);
    joinErrors.add(1, tags);
  }
  return successful;
}

export function current(config, user, assignment) {
  pollingRequests.add(1, metricTags('current', 'success', assignment.product_group));
  const response = http.get(
    `${config.baseURL}/api/v1/products/${assignment.product_id}/queue-entry`,
    {
      headers: { 'X-User-ID': user.id },
      tags: { name: currentName, operation: 'current', product_group: assignment.product_group },
      responseCallback: http.expectedStatuses(200),
    },
  );
  const entry = response.status === 200 ? parseEntry(response) : null;
  const success = entry !== null && validEntry(entry);
  const result = success ? 'success' : 'unexpected_error';
  const tags = metricTags('current', result, assignment.product_group);
  currentDuration.add(response.timings.duration, tags);
  currentSuccess.add(success, tags);
  unexpectedRequestFailureRate.add(!success, tags);
  if (success) {
    recordState(entry, tags);
  } else {
    currentErrors.add(1, tags);
    recordUnexpectedStatus(response, tags);
  }
  return { response, entry, success };
}

export function startCheckout(config, user, assignment, attemptID) {
  const response = http.post(
    `${config.baseURL}/api/v1/queue-attempts/${attemptID}/checkout`,
    null,
    {
      headers: { 'X-User-ID': user.id },
      tags: { name: checkoutName, operation: 'checkout', product_group: assignment.product_group },
      responseCallback: http.expectedStatuses(200),
    },
  );
  const entry = response.status === 200 ? parseEntry(response) : null;
  const success = entry !== null && validEntry(entry, false) && entry.state === 'checkout';
  const tags = metricTags('checkout', success ? 'success' : 'unexpected_error', assignment.product_group);
  checkoutDuration.add(response.timings.duration, tags);
  checkoutSuccess.add(success, tags);
  unexpectedRequestFailureRate.add(!success, tags);
  if (success) {
    recordState(entry, tags);
  } else {
    actionErrors.add(1, tags);
    recordUnexpectedStatus(response, tags);
  }
  return { response, entry, success };
}

export function cancelPurchase(config, user, assignment) {
  const response = http.del(
    `${config.baseURL}/api/v1/products/${assignment.product_id}/queue-entry`,
    null,
    {
      headers: { 'X-User-ID': user.id },
      tags: { name: cancelName, operation: 'cancel', product_group: assignment.product_group },
      responseCallback: http.expectedStatuses(204),
    },
  );
  const success = response.status === 204;
  const tags = metricTags('cancel', success ? 'success' : 'unexpected_error', assignment.product_group);
  cancelDuration.add(response.timings.duration, tags);
  cancelSuccess.add(success, tags);
  unexpectedRequestFailureRate.add(!success, tags);
  if (!success) {
    actionErrors.add(1, tags);
    recordUnexpectedStatus(response, tags);
  }
  return { response, success };
}

export function completePayment(config, assignment, attemptID) {
  const response = http.post(
    `${config.baseURL}/internal/v1/payment-events`,
    JSON.stringify({
      provider: 'goodqueue-loadtest',
      event_id: assignment.payment_event_id,
      attempt_id: attemptID,
      outcome: 'succeeded',
      payment_reference: assignment.payment_reference,
    }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { name: paymentName, operation: 'payment', product_group: assignment.product_group },
      responseCallback: http.expectedStatuses(200, 202),
    },
  );
  const body = parseEntry(response);
  const success = (response.status === 200 && body !== null && ['accepted', 'already_accepted'].includes(body.code))
    || (response.status === 202 && body !== null && body.code === 'processing');
  const tags = metricTags('payment', success ? 'success' : 'unexpected_error', assignment.product_group);
  paymentDuration.add(response.timings.duration, tags);
  paymentSuccess.add(success, tags);
  unexpectedRequestFailureRate.add(!success, tags);
  if (!success) {
    actionErrors.add(1, tags);
    recordUnexpectedStatus(response, tags);
  }
  return { response, success, code: body && body.code };
}

function classifyJoin(response) {
  if (response.status === 200 || response.status === 201) {
    const entry = parseEntry(response);
    const success = entry !== null && validEntry(entry);
    return { response, entry, success, result: success ? 'success' : 'unexpected_error' };
  }
  const code = parseErrorCode(response);
  if ((response.status === 409 || response.status === 410) && expectedJoinErrors.has(code)) {
    return { response, entry: null, success: false, result: 'expected_error', errorCode: code };
  }
  return { response, entry: null, success: false, result: 'unexpected_error', errorCode: code };
}

function parseEntry(response) {
  try {
    return response.json();
  } catch (_) {
    return null;
  }
}

function parseErrorCode(response) {
  try {
    const body = response.json();
    return body && body.error && body.error.code;
  } catch (_) {
    return '';
  }
}

function validEntry(entry, requireTotalWaiting = true) {
  if (
    entry === null || typeof entry.attempt_id !== 'string' || typeof entry.product_id !== 'string'
    || !queueStates.has(entry.state) || !Number.isInteger(entry.queue_sequence) || entry.queue_sequence < 1
    || (requireTotalWaiting && (!Number.isInteger(entry.total_waiting) || entry.total_waiting < 0))
  ) {
    return false;
  }
  if (entry.state === 'waiting') {
    return Number.isInteger(entry.position) && entry.position >= 1
      && Number.isInteger(entry.position_ahead) && entry.position_ahead >= 0
      && entry.position === entry.position_ahead + 1 && entry.total_waiting >= entry.position;
  }
  return true;
}
