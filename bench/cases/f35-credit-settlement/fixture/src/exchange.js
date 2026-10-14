import { nonnegative, positive, checkedMultiply } from './quantity.js';

export function convert(credits, numerator, denominator) {
  nonnegative(credits, 'credits');
  positive(numerator, 'numerator');
  positive(denominator, 'denominator');
  return Math.round(checkedMultiply(credits, numerator) / denominator);
}

export function formatCredits(credits, symbol = 'cr') {
  nonnegative(credits, 'credits');
  return `${credits.toLocaleString('en-US')} ${symbol}`;
}

export function parseCredits(text) {
  if (typeof text !== 'string' || !/^[0-9]+$/.test(text)) {
    throw new TypeError('decimal integer required');
  }
  return nonnegative(Number(text), 'credits');
}

export function distribute(credits, count) {
  nonnegative(credits, 'credits');
  positive(count, 'count');
  const base = Math.floor(credits / count);
  const remainder = credits % count;
  return Array.from({ length: count }, (_, index) => base + (index < remainder ? 1 : 0));
}

export function percentage(credits, basisPoints) {
  nonnegative(credits, 'credits');
  nonnegative(basisPoints, 'basisPoints');
  return Math.round(checkedMultiply(credits, basisPoints) / 10000);
}
