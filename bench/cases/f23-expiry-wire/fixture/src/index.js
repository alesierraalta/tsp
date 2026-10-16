export function decode(payload) {
  const { version, expires } = JSON.parse(payload);
  if (version !== 1 && version !== 2) throw new RangeError("unsupported version");
  return { expiresAt: expires };
}

export function encode(record, version = 2) {
  if (version !== 1 && version !== 2) throw new RangeError("unsupported version");
  const expires = record.expiresAt;
  return JSON.stringify({ version, expires });
}
