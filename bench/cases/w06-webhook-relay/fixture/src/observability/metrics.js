export class Metrics {
  constructor(options = {}) {
    this.counters = new Map();
    this.gauges = new Map();
    this.timings = new Map();
    this.clock = options.clock ?? (() => Date.now());
  }

  increment(name, by = 1) {
    const key = String(name);
    this.counters.set(key, (this.counters.get(key) ?? 0) + by);
    return this.counters.get(key);
  }

  gauge(name, value) {
    this.gauges.set(String(name), Number(value));
    return Number(value);
  }

  observe(name, ms) {
    const key = String(name);
    const list = this.timings.get(key) ?? [];
    list.push(Number(ms));
    this.timings.set(key, list);
    return list.length;
  }

  time(name, run) {
    const started = this.clock();
    const value = run();
    if (value && typeof value.then === 'function') {
      return value.finally(() => this.observe(name, this.clock() - started));
    }
    this.observe(name, this.clock() - started);
    return value;
  }

  counter(name) {
    return this.counters.get(String(name)) ?? 0;
  }

  snapshot() {
    const timings = {};
    for (const [name, values] of this.timings.entries()) {
      timings[name] = summarize(values);
    }
    return {
      counters: Object.fromEntries([...this.counters.entries()].sort()),
      gauges: Object.fromEntries([...this.gauges.entries()].sort()),
      timings,
    };
  }

  reset() {
    this.counters.clear();
    this.gauges.clear();
    this.timings.clear();
  }

  render() {
    const snapshot = this.snapshot();
    const lines = [];
    for (const [name, value] of Object.entries(snapshot.counters)) lines.push(`counter ${name} ${value}`);
    for (const [name, value] of Object.entries(snapshot.gauges)) lines.push(`gauge ${name} ${value}`);
    for (const [name, stats] of Object.entries(snapshot.timings)) {
      lines.push(`timer ${name} count=${stats.count} mean=${stats.mean}`);
    }
    return lines.join('\n');
  }
}

export function summarize(values) {
  if (values.length === 0) return { count: 0, mean: 0, min: 0, max: 0 };
  const total = values.reduce((sum, value) => sum + value, 0);
  return {
    count: values.length,
    mean: Math.round((total / values.length) * 100) / 100,
    min: Math.min(...values),
    max: Math.max(...values),
  };
}

export function mergeMetrics(target, source) {
  for (const [name, value] of source.counters.entries()) target.increment(name, value);
  for (const [name, value] of source.gauges.entries()) target.gauge(name, value);
  return target;
}
