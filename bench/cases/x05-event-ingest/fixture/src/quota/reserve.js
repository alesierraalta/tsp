import { newId } from '../util/ids.js';
import { tooLarge, badRequest } from '../util/errors.js';
import { assertWithinLimits, limitsFor } from '../ingest/limits.js';

export class ReservationBook {
  constructor(options = {}) {
    this.plans = options.plans;
    this.clock = options.clock ?? (() => new Date());
    this.reservations = new Map();
  }

  reserve(tenant, request = {}) {
    const plan = this.plans.for(tenant);
    const rows = Number(request.rows ?? 0);
    const samples = Number(request.samples ?? 0);
    assertWithinLimits(plan, { rows, samples });
    const slots = new Float64Array(samples);
    const reservation = {
      id: newId('rsv'),
      tenant,
      plan: plan.name,
      rows,
      samples,
      slots: slots.length,
      created_at: new Date(this.clock()).toISOString(),
      released_at: null,
    };
    this.reservations.set(reservation.id, reservation);
    return reservation;
  }

  planCapacity(tenant, request = {}) {
    const plan = this.plans.for(tenant);
    const rows = Number(request.rows ?? 0);
    const samples = Number(request.samples ?? 0);
    if (!Number.isInteger(rows) || rows < 0) throw badRequest('invalid_rows', 'rows must be a whole number');
    if (rows > limitsFor(plan).max_rows) {
      throw badRequest('reservation_too_large', `${rows} rows exceed the ${plan.name} plan`, { limit: limitsFor(plan).max_rows });
    }
    const slots = new Float64Array(samples);
    const reservation = {
      id: newId('rsv'),
      tenant,
      plan: plan.name,
      rows,
      samples,
      slots: slots.length,
      created_at: new Date(this.clock()).toISOString(),
      released_at: null,
    };
    this.reservations.set(reservation.id, reservation);
    return reservation;
  }

  release(id) {
    const reservation = this.reservations.get(String(id));
    if (!reservation) return false;
    reservation.released_at = new Date(this.clock()).toISOString();
    reservation.slots = 0;
    return true;
  }

  get(id) {
    return this.reservations.get(String(id));
  }

  list(tenant) {
    return [...this.reservations.values()]
      .filter((reservation) => tenant === undefined || reservation.tenant === tenant)
      .map((reservation) => ({
        id: reservation.id,
        tenant: reservation.tenant,
        rows: reservation.rows,
        samples: reservation.samples,
        released_at: reservation.released_at,
      }));
  }

  open(tenant) {
    return [...this.reservations.values()].filter(
      (reservation) => reservation.released_at === null && (tenant === undefined || reservation.tenant === tenant),
    );
  }

  usedSlots(tenant) {
    return this.open(tenant).reduce((total, reservation) => total + reservation.slots, 0);
  }

  assertCapacity(tenant) {
    const plan = this.plans.for(tenant);
    const used = this.usedSlots(tenant);
    if (used > limitsFor(plan).max_samples) {
      throw tooLarge(`${used} reserved slots exceed the ${plan.name} plan`);
    }
    return used;
  }

  stats() {
    return {
      reservations: this.reservations.size,
      open: this.open().length,
      slots: this.usedSlots(),
    };
  }
}
