export function roundMinor(value) {
  return Math.sign(value) * Math.round(Math.abs(value));
}

export function quoteAdjustment(netMinor, rateBasisPoints) {
  const taxMinor = roundMinor(Math.abs(netMinor) * rateBasisPoints / 10000);
  return { netMinor, taxMinor, grossMinor: netMinor + taxMinor };
}
