import exec from 'k6/execution';
import http from 'k6/http';
import { sleep } from 'k6';

import { effectiveConfig, loadConfig } from './lib/config.js';
import { loadData, seededRandom, userForVU } from './lib/data.js';
import { current, duplicateJoin, join } from './lib/requests.js';

const config = loadConfig();
const data = loadData(open(config.dataFile), config);

export const options = {
  discardResponseBodies: false,
  tags: {
    testid: config.runID,
    profile: config.profile,
    loadtest_scenario: config.scenario,
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  // Excluding the raw URL prevents product UUIDs from creating time-series cardinality.
  // The fixed name tag below is the endpoint dimension used for HTTP aggregation.
  systemTags: [
    'status', 'method', 'name', 'group', 'check', 'error', 'error_code',
    'expected_response', 'scenario', 'proto', 'subproto', 'tls_version',
  ],
  scenarios: {
    queue_join_polling: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: config.rampDuration, target: config.users },
        { duration: config.pollDuration, target: config.users },
        { duration: '1s', target: 0 },
      ],
      gracefulRampDown: config.pollInterval,
    },
  },
  thresholds: {
    unexpected_5xx: ['count==0'],
    unexpected_request_failure_rate: ['rate<0.01'],
    join_duration: ['p(95)<1000', 'p(99)<2000'],
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
    sleep(config.pollMilliseconds / 1000);
    return;
  }
  completedMainIteration = true;

  const vuID = exec.vu.idInTest;
  const user = userForVU(data, vuID);
  const random = seededRandom(config.randomSeed + vuID * 2654435761);
  const joined = [];

  for (const assignment of user.assignments) {
    const result = join(config, user, assignment);
    if (!result.success) {
      continue;
    }
    joined.push(assignment);
    if (assignment.duplicate_join) {
      duplicateJoin(config, user, assignment, result.entry);
    }
  }

  const pollingDeadline = Date.now() + config.pollMilliseconds;
  while (joined.length > 0 && Date.now() < pollingDeadline) {
    const jitter = 0.8 + random() * 0.4;
    const delayMilliseconds = config.pollIntervalMilliseconds * jitter;
    if (Date.now() + delayMilliseconds > pollingDeadline) {
      break;
    }
    sleep(delayMilliseconds / 1000);
    for (const assignment of joined) {
      current(config, user, assignment);
    }
  }
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
    `GoodQueue load test: run=${config.runID} profile=${config.profile}`,
    `users=${config.users} products=${config.products} products_per_user=${config.productsPerUser}`,
    `ramp=${config.rampDuration} poll_interval=${config.pollInterval} poll_duration=${config.pollDuration}`,
    '',
  ];
  const names = [
    'http_reqs', 'http_req_failed', 'join_duration', 'current_duration', 'join_success',
    'current_success', 'duplicate_join_success', 'unexpected_request_failure_rate',
    'unexpected_4xx', 'unexpected_5xx', 'polling_requests', 'dropped_iterations',
  ];
  for (const name of names) {
    const metric = summary.metrics[name];
    if (metric !== undefined) {
      lines.push(`${name}: ${JSON.stringify(metric.values)}`);
    }
  }
  return `${lines.join('\n')}\n`;
}
