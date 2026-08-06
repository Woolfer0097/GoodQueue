import http from 'k6/http';

import {
  currentDuration,
  currentErrors,
  currentSuccess,
  duplicateJoinSuccess,
  joinDuration,
  joinErrors,
  joinSuccess,
  metricTags,
  pollingRequests,
  recordState,
  recordUnexpectedStatus,
  unexpectedRequestFailureRate,
} from './metrics.js';

const joinName = 'POST /api/v1/products/:productID/queue-entries';
const currentName = 'GET /api/v1/products/:productID/queue-entry';
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

function validEntry(entry) {
  if (
    entry === null || typeof entry.attempt_id !== 'string' || typeof entry.product_id !== 'string'
    || !queueStates.has(entry.state) || !Number.isInteger(entry.queue_sequence) || entry.queue_sequence < 1
    || !Number.isInteger(entry.total_waiting) || entry.total_waiting < 0
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
