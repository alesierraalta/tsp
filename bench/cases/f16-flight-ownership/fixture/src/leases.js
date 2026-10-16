export class LeasePool {
  constructor() {
    this.slots = new Map();
  }

  acquire(key) {
    const slot = this.slots.get(key) ?? { busy: false, generation: 0 };
    if (slot.busy) return null;
    slot.generation = 1 - slot.generation;
    slot.busy = true;
    this.slots.set(key, slot);
    return { key, generation: slot.generation };
  }

  release(ticket) {
    const slot = this.slots.get(ticket.key);
    if (!slot || !slot.busy || slot.generation !== ticket.generation) return false;
    slot.busy = false;
    return true;
  }
}
