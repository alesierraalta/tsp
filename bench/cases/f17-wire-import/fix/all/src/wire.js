import { Buffer } from 'node:buffer';

export function packRows(rows) {
  const payload = JSON.stringify(rows);
  return { count: rows.length, payload };
}

export function unpackRows(packet) {
  return JSON.parse(packet.payload).slice(0, packet.count);
}

export function readText(bytes) {
  return Buffer.from(bytes).toString('utf8');
}
