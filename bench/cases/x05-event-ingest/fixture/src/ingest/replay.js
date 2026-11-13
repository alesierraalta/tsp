import { badRequest } from '../util/errors.js';
import { assertWithinLimits } from './limits.js';
import { parseNdjson } from './ndjson.js';
import { nowIso, toIso } from '../util/time.js';

export class Replayer {
  constructor(options = {}) {
    this.intake = options.intake;
    this.plans = options.plans;
    this.clock = options.clock ?? (() => new Date());
    this.runs = [];
  }

  replay(tenant, text, options = {}) {
    const plan = this.plans.for(tenant);
    const rows = parseNdjson(text);
    assertWithinLimits(plan, {
      rows: rows.length,
      samples: rows.reduce((total, row) => total + (Array.isArray(row?.samples) ? row.samples.length : 0), 0),
      bytes: Buffer.byteLength(text, 'utf8'),
    });
    if (options.dry_run === true) {
      return { dry_run: true, rows: rows.length, samples: rows.reduce((total, row) => total + (Array.isArray(row?.samples) ? row.samples.length : 0), 0) };
    }
    const shifted = rows.map((row) => ({
      ...row,
      timestamp: options.shift_ms ? new Date((toIso(row.timestamp) ? new Date(row.timestamp).getTime() : Date.now()) + options.shift_ms).toISOString() : row.timestamp,
    }));
    const result = this.intake.accept(tenant, shifted);
    const run = {
      id: `rep_${this.runs.length + 1}`,
      tenant,
      rows: rows.length,
      accepted: result.accepted,
      rejected: result.rejected.length,
      at: nowIso(this.clock),
      dry_run: false,
    };
    this.runs.push(run);
    return run;
  }

  replayFile(tenant, fileText, options = {}) {
    if (typeof fileText !== 'string' || fileText.trim() === '') {
      throw badRequest('empty_replay', 'a replay needs at least one line');
    }
    return this.replay(tenant, fileText, options);
  }

  history(tenant) {
    return this.runs.filter((run) => tenant === undefined || run.tenant === tenant);
  }

  stats() {
    return {
      runs: this.runs.length,
      rows: this.runs.reduce((total, run) => total + run.rows, 0),
      rejected: this.runs.reduce((total, run) => total + run.rejected, 0),
    };
  }
}

export function replayPlan(rows, options = {}) {
  const samples = rows.reduce((total, row) => total + (Array.isArray(row?.samples) ? row.samples.length : 0), 0);
  return {
    rows: rows.length,
    samples,
    bytes: Buffer.byteLength(JSON.stringify(rows), 'utf8'),
    chunk: options.chunk ?? 500,
    batches: Math.max(1, Math.ceil(rows.length / (options.chunk ?? 500))),
  };
}

export function shiftTimestamps(rows, milliseconds) {
  return rows.map((row) => ({
    ...row,
    timestamp: new Date((toIso(row.timestamp) ? new Date(row.timestamp).getTime() : Date.now()) + milliseconds).toISOString(),
  }));
}
