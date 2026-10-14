export function chunks(bytes, size) {
  if (!Buffer.isBuffer(bytes)) {
    throw new TypeError('buffer required');
  }
  if (!Number.isSafeInteger(size) || size <= 0) {
    throw new RangeError('invalid chunk size');
  }
  const result = [];
  for (let offset = 0; offset < bytes.length; offset += size) {
    result.push(Buffer.from(bytes.subarray(offset, offset + size)));
  }
  return result;
}

export function assemble(parts) {
  return Buffer.concat(parts.map(part => Buffer.from(part)));
}

export function digest(bytes) {
  let sum = 2166136261;
  for (const byte of bytes) {
    sum ^= byte;
    sum = Math.imul(sum, 16777619) >>> 0;
  }
  return sum.toString(16).padStart(8, '0');
}

export function describe(bytes, size) {
  const parts = chunks(bytes, size);
  return {
    bytes: bytes.length,
    parts: parts.length,
    checksum: digest(bytes),
    largest: parts.reduce((max, part) => Math.max(max, part.length), 0),
  };
}
