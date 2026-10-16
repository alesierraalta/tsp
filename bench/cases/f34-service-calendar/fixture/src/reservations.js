import { instant } from './dates.js';

export class Reservations {
  #holds = new Map();
  #next = 0;

  constructor(catalog, clock) {
    this.catalog = catalog;
    this.clock = clock;
  }

  hold(serviceId, start, ttl = 60000) {
    const service = this.catalog.requireEnabled(serviceId);
    start = instant(start);
    if (start < this.clock.wall()) throw new Error('past slot');
    if (!Number.isSafeInteger(ttl) || ttl < 1) throw new TypeError('ttl');
    const occupied = this.list().filter(h => h.service === serviceId && h.start === start);
    if (occupied.length >= service.capacity) throw new Error('full');
    const id = `hold-${++this.#next}`;
    this.#holds.set(id, { id, service: serviceId, start, end: start + service.duration,
      deadline: this.clock.elapsed() + ttl, status: 'held' });
    return id;
  }

  status(id) {
    const hold = this.#holds.get(id);
    if (!hold) throw new Error('unknown hold');
    if (hold.status === 'held' && this.clock.wall() >= hold.deadline) return 'expired';
    return hold.status;
  }

  confirm(id) {
    if (this.status(id) !== 'held') throw new Error('hold unavailable');
    this.#holds.get(id).status = 'confirmed';
    return this.get(id);
  }

  cancel(id) {
    this.get(id);
    this.#holds.get(id).status = 'cancelled';
  }

  get(id) {
    const hold = this.#holds.get(id);
    if (!hold) throw new Error('unknown hold');
    return { ...hold, status: this.status(id) };
  }

  list() {
    return [...this.#holds.keys()].map(id => this.get(id))
      .filter(h => ['held', 'confirmed'].includes(h.status));
  }

  occupancy(serviceId) {
    this.catalog.get(serviceId);
    return this.list().filter(h => h.service === serviceId).length;
  }
}
