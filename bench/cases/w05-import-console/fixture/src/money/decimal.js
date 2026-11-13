import { roundHalfUp } from '../util/num.js';

const CURRENCY_SYMBOLS = {
  USD: '$',
  EUR: '\u20ac',
  GBP: '\u00a3',
  ARS: 'AR$',
};

export const ZERO = 0;

export function currencySymbol(currency) {
  return CURRENCY_SYMBOLS[currency] ?? '';
}

export function parseCents(input) {
  if (input === null || input === undefined || input === '') return undefined;
  if (typeof input === 'number') {
    if (!Number.isFinite(input)) throw new TypeError(`not a money amount: ${input}`);
    return roundHalfUp(input * 100, 0);
  }
  let text = String(input).trim();
  if (text === '') return undefined;
  let negative = false;
  if (/^\(.*\)$/.test(text)) {
    negative = true;
    text = text.slice(1, -1);
  }
  text = text.replace(/[A-Z]{3}|\$|\u20ac|\u00a3|AR\$|,|\s/g, '');
  if (text.startsWith('-')) {
    negative = !negative;
    text = text.slice(1);
  }
  if (!/^\d+(\.\d{1,4})?$/.test(text)) {
    throw new TypeError(`not a money amount: ${input}`);
  }
  const [whole, fraction = ''] = text.split('.');
  let cents = Number(whole) * 100 + Number(`${fraction}00`.slice(0, 2));
  if (fraction.length > 2 && Number(fraction[2]) >= 5) cents += 1;
  if (!Number.isSafeInteger(cents)) {
    throw new TypeError(`money amount is too large: ${input}`);
  }
  return negative ? -cents : cents;
}

export function formatCents(cents, currency = 'USD', options = {}) {
  if (!Number.isInteger(cents)) throw new TypeError(`cents must be a whole number: ${cents}`);
  const negative = cents < 0;
  const absolute = Math.abs(cents);
  const whole = Math.floor(absolute / 100);
  const fraction = String(absolute % 100).padStart(2, '0');
  const grouped = String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, options.thousands ?? ',');
  const symbol = options.symbol === false ? '' : currencySymbol(currency);
  const spaced = symbol && symbol !== '$' ? `${symbol}\u00a0` : symbol;
  return `${negative ? '-' : ''}${spaced}${grouped}.${fraction}`;
}

export function addCents(...amounts) {
  let total = 0;
  for (const amount of amounts) {
    if (!Number.isInteger(amount)) throw new TypeError(`cents must be whole numbers: ${amount}`);
    total += amount;
  }
  if (!Number.isSafeInteger(total)) throw new TypeError('the money total is out of range');
  return total;
}

export function sumCents(amounts) {
  return addCents(0, ...amounts.filter((amount) => amount !== undefined));
}

export function subtractCents(left, right) {
  return addCents(left, -right);
}

export function multiplyCents(cents, factor) {
  if (!Number.isInteger(cents)) throw new TypeError('cents must be a whole number');
  if (!Number.isFinite(factor)) throw new TypeError('the factor must be a number');
  return roundHalfUp(cents * factor, 0);
}

export function percentOfCents(cents, percent) {
  return multiplyCents(cents, percent / 100);
}

export function splitCents(cents, parts) {
  if (!Number.isInteger(parts) || parts <= 0) throw new TypeError('parts must be a positive integer');
  const base = Math.trunc(cents / parts);
  const remainder = cents - base * parts;
  return Array.from({ length: parts }, (_, index) => base + (index < Math.abs(remainder) ? Math.sign(remainder) : 0));
}

export function taxCents(net, ratePercent) {
  return percentOfCents(net, ratePercent);
}

export function grossCents(net, ratePercent) {
  return addCents(net, taxCents(net, ratePercent));
}

export function compareCents(left, right) {
  return left === right ? 0 : left < right ? -1 : 1;
}

export function clampCents(cents, low, high) {
  return Math.min(high, Math.max(low, cents));
}

export function isZero(cents) {
  return cents === 0;
}

export function parseMoneyCell(value) {
  try {
    return parseCents(value);
  } catch {
    return undefined;
  }
}
