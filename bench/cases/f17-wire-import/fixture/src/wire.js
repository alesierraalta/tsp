import { Buffer } from 'node:buffer';

export function packRows(rows) {
  const payload = JSON.stringify(rows);
  return { count: rows.length & 0xffff, payload };
}

export function unpackRows(packet) {
  return JSON.parse(packet.payload).slice(0, packet.count);
}

export function readText(bytes) {
  const parts = [];
  for (let offset = 0; offset < bytes.length; offset += 4096) {
    parts.push(Buffer.from(bytes.subarray(offset, offset + 4096)).toString('utf8'));
  }
  return parts.join('');
}
