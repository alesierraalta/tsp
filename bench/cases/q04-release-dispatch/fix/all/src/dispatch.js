export class Dispatcher {
  constructor(catalog, { clock = Date.now, retention = 60000 } = {}) {
    this.catalog = catalog;
    this.clock = clock;
    this.retention = retention;
    this.artifacts = new Map();
    this.jobs = new Map();
  }

  publish(id, environment, version, mode = 'durable') {
    const key = JSON.stringify([environment, version]);
    if (!this.artifacts.has(key)) {
      const artifact = this.catalog.find(row => row.environment === environment && row.version === version);
      if (!artifact) return false;
      this.artifacts.set(key, artifact.digest);
    }
    this.jobs.set(id, { artifact: this.artifacts.get(key), mode, created: this.clock() });
    return true;
  }

  collect() {
    const now = this.clock();
    for (const [id, job] of this.jobs) {
      if (job.mode === 'transient' && now - job.created >= this.retention) this.jobs.delete(id);
    }
    return [...this.jobs].map(([id, job]) => ({ id, artifact: job.artifact }));
  }
}
