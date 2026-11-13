import { eventId } from '../util/ids.js';
import { nowIso, toIso } from '../util/time.js';
import { badRequest } from '../util/errors.js';
import { planBatch } from './limits.js';
import { validateEvent } from './schemas.js';
import { isBlank } from '../util/text.js';

export class Intake {
  constructor(options = {}) {
    this.store = options.store;
    this.plans = options.plans;
    this.schemas = options.schemas;
    this.dedupe = options.dedupe;
    this.series = options.series;
    this.clock = options.clock ?? (() => new Date());
    this.batches = 0;
  }

  normalise(tenant, event = {}) {
    if (isBlank(event.name)) throw badRequest('invalid_event', 'an event needs a name');
    const at = toIso(event.timestamp) ?? nowIso(this.clock);
    const id = String(event.id ?? eventId());
    return {
      id,
      tenant,
      name: String(event.name),
      at,
      received_at: nowIso(this.clock),
      properties: { ...(event.properties ?? {}) },
      samples: Array.isArray(event.samples) ? event.samples.map((sample) => Number(sample)) : undefined,
      context: {
        path: event.context?.path ?? null,
        referrer: event.context?.referrer ?? null,
        user: event.context?.user ?? null,
        session: event.context?.session ?? null,
      },
    };
  }

  accept(tenant, events = []) {
    const plan = this.plans.for(tenant);
    const planned = planBatch(plan, { events });
    const accepted = [];
    const rejected = [];
    for (const [index, event] of events.entries()) {
      const type = this.schemas.get(String(event?.name ?? ''));
      const check = validateEvent(type, event ?? {});
      if (!check.ok) {
        rejected.push({ index, problems: check.problems });
        continue;
      }
      const normalised = this.normalise(tenant, event);
      if (this.dedupe?.seen(tenant, normalised.id)) {
        rejected.push({ index, problems: ['duplicate event id'] });
        continue;
      }
      this.dedupe?.remember(tenant, normalised.id);
      this.store.insert('events', normalised.id, normalised);
      this.series?.add(tenant, normalised.name, normalised.at, normalised.samples?.length ?? 1, type?.kind ?? 'custom');
      accepted.push(normalised);
    }
    this.batches += 1;
    return {
      accepted: accepted.length,
      rejected,
      slots: planned.slots.length,
      batch: `bat_${this.batches}`,
    };
  }

  appendLine(tenant, line) {
    if (line.trim() === '') return { accepted: 0, rejected: [] };
    let event;
    try {
      event = JSON.parse(line);
    } catch (error) {
      throw badRequest('invalid_ndjson', `a line is not valid JSON: ${error.message}`);
    }
    return this.accept(tenant, [event]);
  }

  eventsOf(tenant, options = {}) {
    return this.store
      .find('events', (event) => event.tenant === tenant && (options.name === undefined || event.name === options.name))
      .sort((left, right) => (left.at < right.at ? -1 : 1));
  }

  counts(tenant) {
    const events = this.eventsOf(tenant);
    const byName = {};
    for (const event of events) byName[event.name] = (byName[event.name] ?? 0) + 1;
    return { total: events.length, byName };
  }
}
