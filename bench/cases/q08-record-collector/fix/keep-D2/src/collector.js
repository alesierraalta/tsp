'use strict';
class Collector {
  constructor({lookup = keys => keys, release = () => true, log = {read: () => [], write: () => {}}, limit = 2} = {}) {
    this.lookup = lookup;
    this.release = release;
    this.log = log;
    this.limit = Math.max(0, Math.floor(limit) || 0);
    this.cache = new Map();
  }
  records(keys) {
    const missing = [...new Set(keys)].filter(key => !this.cache.has(key));
    if (missing.length) {
      const values = this.lookup(missing);
      missing.forEach((key, index) => this.cache.set(key, values[index]));
    }
    return keys.map(key => this.cache.get(key));
  }
  collect(keys) {
    const entries = this.log.read().slice();
    const added = [];
    for (const key of keys) if (!this.release(key)) added.push(key);
    this.log.write(entries.concat(added.slice(-this.limit)));
  }
}
module.exports = {Collector};
