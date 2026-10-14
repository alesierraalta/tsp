export function roundMinor(value) {
  return Math.round(value);
}

export function quoteAdjustment(netMinor, rateBasisPoints) {
  const taxMinor = roundMinor(netMinor * rateBasisPoints / 10000);
  return { netMinor, taxMinor, grossMinor: netMinor + taxMinor };
}
