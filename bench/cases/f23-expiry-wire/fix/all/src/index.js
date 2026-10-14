export function decode(payload) {
  const { version, expires } = JSON.parse(payload);
  if (version !== 1 && version !== 2) throw new RangeError("unsupported version");
  return { expiresAt: version === 1 ? expires * 1000 : expires };
}

export function encode(record, version = 2) {
  if (version !== 1 && version !== 2) throw new RangeError("unsupported version");
  const expires = version === 1 ? record.expiresAt / 1000 : record.expiresAt;
  return JSON.stringify({ version, expires });
}
