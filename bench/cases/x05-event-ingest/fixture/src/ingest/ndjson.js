import { assertWithinLimits, limitsFor } from './limits.js';
import { badRequest } from '../util/errors.js';

export function splitLines(text) {
  return String(text ?? '')
    .split(/\r?\n/)
    .filter((line) => line.trim() !== '');
}

export function planNdjson(plan, text) {
  const lines = splitLines(text);
  const limits = limitsFor(plan);
  const oversized = lines.filter((line) => Buffer.byteLength(line, 'utf8') > limits.max_line_bytes);
  if (oversized.length > 0) {
    throw badRequest('line_too_large', `${oversized.length} line(s) exceed ${limits.max_line_bytes} bytes`, {
      limit: limits.max_line_bytes,
    });
  }
  return assertWithinLimits(plan, {
    rows: lines.length,
    samples: 0,
    bytes: Buffer.byteLength(text ?? '', 'utf8'),
  });
}

export function parseNdjson(text) {
  return splitLines(text).map((line, index) => {
    try {
      return JSON.parse(line);
    } catch (error) {
      throw badRequest('invalid_ndjson', `line ${index + 1} is not valid JSON: ${error.message}`);
    }
  });
}

export function toNdjson(rows) {
  return rows.map((row) => JSON.stringify(row)).join('\n');
}

export class NdjsonIntake {
  constructor(options = {}) {
    this.intake = options.intake;
    this.plans = options.plans;
  }

  submit(tenant, text) {
    const plan = this.plans.for(tenant);
    planNdjson(plan, text);
    const rows = parseNdjson(text);
    return this.intake.accept(tenant, rows);
  }

  append(tenant, line, options = {}) {
    const plan = this.plans.for(tenant);
    const limits = limitsFor(plan);
    if (Buffer.byteLength(line, 'utf8') > limits.max_line_bytes) {
      throw badRequest('line_too_large', `a line holds at most ${limits.max_line_bytes} bytes`);
    }
    if (options.offset !== undefined && options.offset > limits.max_bytes) {
      throw badRequest('payload_too_large', `an append may not start past ${limits.max_bytes} bytes`);
    }
    return this.intake.appendLine(tenant, line);
  }

  summary(tenant) {
    const events = this.intake.eventsOf(tenant);
    return { rows: events.length, bytes: Buffer.byteLength(toNdjson(events), 'utf8') };
  }
}
