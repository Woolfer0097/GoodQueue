import { SharedArray } from 'k6/data';

export function loadData(raw, config) {
  const parsed = JSON.parse(raw);
  if (parsed.run_id !== config.runID) {
    throw new Error(`data.json run_id ${parsed.run_id} does not match ${config.runID}`);
  }
  if (!Array.isArray(parsed.users) || parsed.users.length !== config.users) {
    throw new Error(`data.json has ${parsed.users && parsed.users.length} users; expected ${config.users}`);
  }
  if (!Array.isArray(parsed.products) || parsed.products.length !== config.products) {
    throw new Error(`data.json has ${parsed.products && parsed.products.length} products; expected ${config.products}`);
  }
  for (const user of parsed.users) {
    if (!Array.isArray(user.assignments) || user.assignments.length !== config.productsPerUser) {
      throw new Error(`user ${user.id} has an invalid assignment count`);
    }
    const distinct = new Set(user.assignments.map((assignment) => assignment.product_id));
    if (distinct.size !== user.assignments.length) {
      throw new Error(`user ${user.id} has duplicate products in the main scenario`);
    }
    if (config.scenario === 'purchase_outcomes') {
      for (const assignment of user.assignments) {
        if (!['purchase', 'cancel', 'ttl'].includes(assignment.planned_outcome)) {
          throw new Error(`user ${user.id} has an invalid planned outcome`);
        }
        if (assignment.planned_outcome === 'purchase'
          && (typeof assignment.payment_event_id !== 'string' || typeof assignment.payment_reference !== 'string')) {
          throw new Error(`user ${user.id} purchase assignment lacks payment identifiers`);
        }
      }
    }
  }
  return {
    users: new SharedArray(`goodqueue-loadtest-users-${config.runID}`, () => parsed.users),
    products: new SharedArray(`goodqueue-loadtest-products-${config.runID}`, () => parsed.products),
  };
}

export function userForVU(data, vuID) {
  const index = vuID - 1;
  if (index < 0 || index >= data.users.length) {
    throw new Error(`VU ${vuID} does not have a seeded user`);
  }
  return data.users[index];
}

export function seededRandom(seed) {
  let state = (Number(seed) >>> 0) || 0x6d2b79f5;
  return function next() {
    state += 0x6d2b79f5;
    let value = state;
    value = Math.imul(value ^ (value >>> 15), value | 1);
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
  };
}
