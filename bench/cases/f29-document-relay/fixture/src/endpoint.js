import { assemble, digest } from './chunks.js';

export class MemoryEndpoint {
  constructor(beforeChunk = async () => {}) {
    this.beforeChunk = beforeChunk;
    this.objects = new Map();
    this.requests = 0;
    this.bytesReceived = 0;
  }

  async replace(key, iterator, attempt) {
    this.requests++;
    const parts = [];
    let position = 0;
    for (let next = iterator.next(); !next.done; next = iterator.next()) {
      await this.beforeChunk({ key, attempt, position, bytes: next.value.length });
      const part = Buffer.from(next.value);
      parts.push(part);
      position++;
      this.bytesReceived += part.length;
    }
    const body = assemble(parts);
    this.objects.set(key, body);
    return { key, bytes: body.length, checksum: digest(body) };
  }

  read(key) {
    const body = this.objects.get(key);
    return body === undefined ? null : Buffer.from(body);
  }

  keys() {
    return [...this.objects.keys()].sort();
  }

  stats() {
    return { requests: this.requests, bytesReceived: this.bytesReceived, objects: this.objects.size };
  }
}
