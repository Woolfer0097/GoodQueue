import exec from 'k6/execution';
import http from 'k6/http';
import { sleep } from 'k6';

import { effectiveConfig, loadConfig } from './lib/config.js';
import { loadData, seededRandom, userForVU } from './lib/data.js';
import {
  cancelledOutcomes,
  checkoutExpiredOutcomes,
  outcomeMismatches,
  purchasedOutcomes,
  queueRejectedOutcomes,
  soldOutOutcomes,
  unresolvedOutcomes,
} from './lib/metrics.js';
import {
  cancelPurchase,
  completePayment,
  current,
  duplicateJoin,
  join,
  startCheckout,
} from './lib/requests.js';

const config = loadConfig();
if (config.scenario !== 'purchase_outcomes') {
  throw new Error('queue-purchase-outcomes.js requires LOADTEST_SCENARIO=purchase_outcomes');
}
const data = loadData(open(config.dataFile), config);
const expectedTerminalState = { purchase: 'purchased', cancel: 'cancelled', ttl: 'checkout_expired' };

export const options = {
  discardResponseBodies: false,
  tags: {
    testid: config.runID,
    profile: config.profile,
    loadtest_scenario: config.scenario,
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  systemTags: [
    'status', 'method', 'name', 'group', 'check', 'error', 'error_code',
    'expected_response', 'scenario', 'proto', 'subproto', 'tls_version',
  ],
  scenarios: {
    purchase_outcomes: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: config.rampDuration, target: config.users },
        { duration: config.outcomeTimeout, target: config.users },
        { duration: '1s', target: 0 },
      ],
      gracefulRampDown: config.pollInterval,
    },
  },
  thresholds: {
    unexpected_5xx: ['count==0'],
    unexpected_request_failure_rate: ['rate<0.01'],
    outcome_mismatches: ['count==0'],
    unresolved_outcomes: ['count==0'],
    checkout_duration: ['p(95)<1000', 'p(99)<2000'],
    cancel_duration: ['p(95)<1000', 'p(99)<2000'],
    payment_duration: ['p(95)<1000', 'p(99)<2000'],
    current_duration: ['p(95)<500', 'p(99)<1000'],
    dropped_iterations: ['count==0'],
  },
};

let completedMainIteration = false;

export function setup() {
  const response = http.get(`${config.baseURL}/readyz`, { tags: { name: 'GET /readyz', operation: 'ready' } });
  if (response.status !== 200) {
    throw new Error(`backend is not ready: HTTP ${response.status}`);
  }
}

export default function () {
  if (completedMainIteration) {
    sleep(config.outcomeTimeoutMilliseconds / 1000);
    return;
  }
  completedMainIteration = true;

  const vuID = exec.vu.idInTest;
  const user = userForVU(data, vuID);
  const random = seededRandom(config.randomSeed + vuID * 2654435761);
  const pending = [];

  for (const assignment of user.assignments) {
    const joined = join(config, user, assignment);
    if (!joined.success) {
      const actualOutcome = joined.errorCode === 'sold_out'
        ? 'sold_out'
        : (joined.result === 'expected_error' ? 'queue_rejected' : 'unresolved');
      if (joined.errorCode === 'sold_out') {
        soldOutOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
      } else if (joined.result === 'expected_error') {
        queueRejectedOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
      } else {
        unresolvedOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
      }
      emitOutcome(user, assignment, {
        operation: 'POST /api/v1/products/{productID}/queue-entries',
        http_status: joined.response.status,
        final_state: joined.errorCode || 'join_error',
        actual_outcome: actualOutcome,
        technical_error: joined.result === 'unexpected_error' ? 'unexpected join response' : '',
      });
      continue;
    }
    if (assignment.duplicate_join) {
      duplicateJoin(config, user, assignment, joined.entry);
    }
    pending.push({
      assignment, entry: joined.entry, actionSent: false, done: false,
      operation: '', httpStatus: null, technicalError: '', emitted: false,
    });
  }

  const deadline = Date.now() + config.outcomeTimeoutMilliseconds;
  while (pending.some((item) => !item.done) && Date.now() < deadline) {
    for (const item of pending) {
      if (item.done) {
        continue;
      }
      advanceItem(item, user);
    }
    if (pending.every((item) => item.done)) {
      break;
    }
    const jitter = 0.8 + random() * 0.4;
    const delayMilliseconds = config.pollIntervalMilliseconds * jitter;
    if (Date.now() + delayMilliseconds >= deadline) {
      break;
    }
    sleep(delayMilliseconds / 1000);
    for (const item of pending) {
      if (item.done) {
        continue;
      }
      const result = current(config, user, item.assignment);
      if (result.success) {
        item.entry = result.entry;
      }
    }
  }

  for (const item of pending) {
    if (!item.done) {
      unresolvedOutcomes.add(1, {
        planned_outcome: item.assignment.planned_outcome,
        final_state: item.entry && item.entry.state ? item.entry.state : 'request_error',
      });
      emitItemOutcome(item, user, 'unresolved', item.entry && item.entry.state ? item.entry.state : 'request_error', 'outcome timeout reached');
    }
  }
}

function advanceItem(item, user) {
  const assignment = item.assignment;
  const state = item.entry && item.entry.state;
  if (state === 'waiting') {
    return;
  }
  if (state === 'invited') {
    const checkout = startCheckout(config, user, assignment, item.entry.attempt_id);
    if (checkout.success) {
      item.entry = checkout.entry;
    }
    return;
  }
  if (state === 'checkout' && !item.actionSent) {
    if (assignment.planned_outcome === 'purchase') {
      const payment = completePayment(config, assignment, item.entry.attempt_id);
      item.actionSent = payment.success;
      item.operation = 'POST /internal/v1/payment-events';
      item.httpStatus = payment.response.status;
      if (!payment.success) {
        item.technicalError = `payment callback failed with HTTP ${payment.response.status}`;
      }
    } else if (assignment.planned_outcome === 'cancel') {
      const cancellation = cancelPurchase(config, user, assignment);
      item.actionSent = cancellation.success;
      item.operation = 'DELETE /api/v1/products/{productID}/queue-entry';
      item.httpStatus = cancellation.response.status;
      if (!cancellation.success) {
        item.technicalError = `cancel failed with HTTP ${cancellation.response.status}`;
      }
    } else {
      // TTL is deliberate inactivity. Polling continues until the backend moves
      // the attempt after its real checkout deadline.
      item.actionSent = true;
      item.operation = 'wait_checkout_ttl';
    }
    return;
  }
  if (['purchased', 'cancelled', 'checkout_expired'].includes(state)) {
    const expected = expectedTerminalState[assignment.planned_outcome];
    if (state !== expected) {
      outcomeMismatches.add(1, { planned_outcome: assignment.planned_outcome, final_state: state });
    }
    if (state === 'purchased') {
      purchasedOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
    } else if (state === 'cancelled') {
      cancelledOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
    } else {
      checkoutExpiredOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
    }
    item.done = true;
    emitItemOutcome(item, user, state, state, item.technicalError);
    return;
  }
  if (state === 'sold_out') {
    soldOutOutcomes.add(1, { planned_outcome: assignment.planned_outcome });
    item.done = true;
    emitItemOutcome(item, user, 'sold_out', state, item.technicalError);
    return;
  }
  if (['invite_expired', 'payment_failed'].includes(state)) {
    outcomeMismatches.add(1, { planned_outcome: assignment.planned_outcome, final_state: state });
    unresolvedOutcomes.add(1, { planned_outcome: assignment.planned_outcome, final_state: state });
    item.done = true;
    emitItemOutcome(item, user, 'unresolved', state, `unexpected terminal state ${state}`);
  }
}

function emitItemOutcome(item, user, actualOutcome, finalState, technicalError) {
  if (item.emitted) {
    return;
  }
  item.emitted = true;
  emitOutcome(user, item.assignment, {
    attempt_id: item.entry && item.entry.attempt_id,
    operation: item.operation || 'GET /api/v1/products/{productID}/queue-entry',
    http_status: item.httpStatus,
    payment_event_id: item.assignment.planned_outcome === 'purchase' ? item.assignment.payment_event_id : '',
    final_state: finalState,
    actual_outcome: actualOutcome,
    technical_error: technicalError || '',
  });
}

function emitOutcome(user, assignment, outcome) {
  console.info(`GOODQUEUE_OUTCOME ${JSON.stringify({
    run_id: config.runID,
    external_user_id: user.id,
    product_id: assignment.product_id,
    ...outcome,
  })}`);
}

export function handleSummary(summary) {
  const directory = `${config.resultsDir}/${config.runID}`;
  return {
    [`${directory}/summary.json`]: `${JSON.stringify(summary, null, 2)}\n`,
    [`${directory}/summary.txt`]: textSummary(summary),
    [`${directory}/effective-config.json`]: `${JSON.stringify(effectiveConfig(config), null, 2)}\n`,
    stdout: textSummary(summary),
  };
}

function textSummary(summary) {
  const lines = [
    `GoodQueue purchase outcomes: run=${config.runID} profile=${config.profile}`,
    `users=${config.users} products=${config.products} products_per_user=${config.productsPerUser}`,
    `ramp=${config.rampDuration} poll_interval=${config.pollInterval} outcome_timeout=${config.outcomeTimeout}`,
    '',
  ];
  const names = [
    'http_reqs', 'http_req_failed', 'join_duration', 'current_duration', 'checkout_duration',
    'cancel_duration', 'payment_duration', 'join_success', 'current_success', 'checkout_success',
    'cancel_success', 'payment_success', 'unexpected_request_failure_rate', 'outcome_mismatches',
    'unresolved_outcomes', 'purchased_outcomes', 'cancelled_outcomes', 'checkout_expired_outcomes',
    'queue_rejected_outcomes', 'sold_out_outcomes', 'unexpected_4xx', 'unexpected_5xx', 'dropped_iterations',
  ];
  for (const name of names) {
    const metric = summary.metrics[name];
    if (metric !== undefined) {
      lines.push(`${name}: ${JSON.stringify(metric.values)}`);
    }
  }
  return `${lines.join('\n')}\n`;
}
