const profiles = {
  smoke: { users: 10, products: 5, productsPerUser: 2, rampDuration: '30s', pollDuration: '1m' },
  medium: { users: 100, products: 20, productsPerUser: 5, rampDuration: '1m', pollDuration: '2m' },
  main: { users: 1000, products: 100, productsPerUser: 10, rampDuration: '5m', pollDuration: '5m' },
};

function env(name, fallback) {
  const value = __ENV[name];
  return value === undefined || String(value).trim() === '' ? fallback : String(value).trim();
}

function integer(name, fallback) {
  const raw = env(name, String(fallback));
  if (!/^-?\d+$/.test(raw)) {
    throw new Error(`${name} must be an integer`);
  }
  return Number.parseInt(raw, 10);
}

function booleanValue(name, fallback) {
  const raw = env(name, String(fallback)).toLowerCase();
  if (raw !== 'true' && raw !== 'false') {
    throw new Error(`${name} must be true or false`);
  }
  return raw === 'true';
}

export function durationMilliseconds(raw, name) {
  const input = String(raw).trim();
  const pattern = /(\d+(?:\.\d+)?)(ms|s|m|h)/g;
  const factors = { ms: 1, s: 1000, m: 60000, h: 3600000 };
  let total = 0;
  let consumed = '';
  let match;
  while ((match = pattern.exec(input)) !== null) {
    total += Number.parseFloat(match[1]) * factors[match[2]];
    consumed += match[0];
  }
  if (consumed !== input || total <= 0) {
    throw new Error(`${name} must be a positive duration using ms, s, m, or h`);
  }
  return total;
}

export function loadConfig() {
  const profileName = env('LOADTEST_PROFILE', 'smoke').toLowerCase();
  const profile = profiles[profileName];
  if (profile === undefined) {
    throw new Error('LOADTEST_PROFILE must be one of smoke, medium, main');
  }
  const config = {
    profile: profileName,
    baseURL: env('LOADTEST_BASE_URL', 'http://localhost:8080').replace(/\/+$/, ''),
    runID: env('LOADTEST_RUN_ID', 'local'),
    randomSeed: integer('LOADTEST_RANDOM_SEED', 42),
    users: integer('LOADTEST_USERS', profile.users),
    products: integer('LOADTEST_PRODUCTS', profile.products),
    productsPerUser: integer('LOADTEST_PRODUCTS_PER_USER', profile.productsPerUser),
    rampDuration: env('LOADTEST_RAMP_DURATION', profile.rampDuration),
    pollInterval: env('LOADTEST_POLL_INTERVAL', '10s'),
    pollDuration: env('LOADTEST_POLL_DURATION', profile.pollDuration),
    queueCapacity: integer('LOADTEST_QUEUE_CAPACITY', 1000),
    duplicateJoinPercent: integer('LOADTEST_DUPLICATE_JOIN_PERCENT', 10),
    minStock: integer('LOADTEST_MIN_STOCK', 1),
    maxStock: integer('LOADTEST_MAX_STOCK', 20),
    cleanupBeforeSeed: booleanValue('LOADTEST_CLEANUP_BEFORE_SEED', false),
    keepData: booleanValue('LOADTEST_KEEP_DATA', true),
    dataFile: env('LOADTEST_DATA_FILE', '../generated/data.json'),
    resultsDir: env('LOADTEST_RESULTS_DIR', 'loadtest/results'),
  };
  config.rampMilliseconds = durationMilliseconds(config.rampDuration, 'LOADTEST_RAMP_DURATION');
  config.pollIntervalMilliseconds = durationMilliseconds(config.pollInterval, 'LOADTEST_POLL_INTERVAL');
  config.pollMilliseconds = durationMilliseconds(config.pollDuration, 'LOADTEST_POLL_DURATION');
  validate(config);
  return config;
}

function validate(config) {
  if (!/^[A-Za-z0-9][A-Za-z0-9.-]{0,39}$/.test(config.runID)) {
    throw new Error('LOADTEST_RUN_ID contains unsafe characters');
  }
  if (!/^https?:\/\/[^/]+/.test(config.baseURL)) {
    throw new Error('LOADTEST_BASE_URL must be an absolute HTTP(S) URL');
  }
  if (config.users <= 0 || config.products <= 0 || config.productsPerUser <= 0) {
    throw new Error('user and product counts must be positive');
  }
  if (config.productsPerUser > config.products) {
    throw new Error('LOADTEST_PRODUCTS_PER_USER must not exceed LOADTEST_PRODUCTS');
  }
  if (config.queueCapacity < 1 || config.queueCapacity > 1000) {
    throw new Error('LOADTEST_QUEUE_CAPACITY must be between 1 and 1000');
  }
  if (config.users * config.productsPerUser > config.products * config.queueCapacity) {
    throw new Error('requested links exceed LOADTEST_QUEUE_CAPACITY across all products');
  }
  if (config.duplicateJoinPercent < 0 || config.duplicateJoinPercent > 100) {
    throw new Error('LOADTEST_DUPLICATE_JOIN_PERCENT must be between 0 and 100');
  }
  if (config.minStock < 1 || config.maxStock < config.minStock) {
    throw new Error('invalid LOADTEST_MIN_STOCK/LOADTEST_MAX_STOCK range');
  }
}

export function effectiveConfig(config) {
  return {
    profile: config.profile,
    base_url: config.baseURL,
    run_id: config.runID,
    random_seed: config.randomSeed,
    users: config.users,
    products: config.products,
    products_per_user: config.productsPerUser,
    ramp_duration: config.rampDuration,
    poll_interval: config.pollInterval,
    poll_duration: config.pollDuration,
    queue_capacity: config.queueCapacity,
    duplicate_join_percent: config.duplicateJoinPercent,
    min_stock: config.minStock,
    max_stock: config.maxStock,
    keep_data: config.keepData,
	cleanup_before_seed: config.cleanupBeforeSeed,
    data_file: config.dataFile,
    results_dir: config.resultsDir,
  };
}
