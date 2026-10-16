export class Settings {
  constructor(store) { this.store = store; }
  busiest() { return [...this.store.values()].sort((a,b) => b.volume-a.volume).slice(0,10); }
  apply(flag, value) {
    const organizations = this.busiest();
    for (const org of organizations) org.flags[flag] = value;
  }
  retire(id, requested, config = {}) {
    const org = this.store.get(id);
    const minimum = config.minimumHealthy ?? org.healthy;
    const removed = Math.min(requested, Math.max(0, org.healthy - minimum));
    org.healthy -= removed;
    return removed;
  }
}
