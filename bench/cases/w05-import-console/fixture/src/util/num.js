export function parseNumber(value, options = {}) {
  if (value === null || value === undefined || value === '') return undefined;
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new TypeError(`not a finite number: ${value}`);
    return value;
  }
  let text = String(value).trim();
  if (options.parenthesesNegative && /^\(.*\)$/.test(text)) {
    text = `-${text.slice(1, -1)}`;
  }
  text = text.replace(options.thousands ?? /,/g, '');
  if (options.percent && text.endsWith('%')) {
    text = text.slice(0, -1);
    const parsed = Number(text);
    if (!Number.isFinite(parsed)) throw new TypeError(`not a number: ${value}`);
    return parsed / 100;
  }
  if (!/^-?\d+(\.\d+)?$/.test(text)) {
    throw new TypeError(`not a number: ${value}`);
  }
  return Number(text);
}

export function roundHalfUp(value, places = 0) {
  if (!Number.isFinite(value)) throw new TypeError('roundHalfUp expects a finite number');
  const factor = 10 ** places;
  const scaled = value * factor;
  const rounded = scaled >= 0 ? Math.floor(scaled + 0.5) : Math.ceil(scaled - 0.5);
  return rounded / factor;
}

export function clamp(value, low, high) {
  return Math.min(high, Math.max(low, value));
}

export function percentOf(part, whole) {
  if (whole === 0) return 0;
  return (part / whole) * 100;
}

export function formatInt(value) {
  return String(Math.trunc(value)).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function formatPercent(value, places = 1) {
  return `${roundHalfUp(value, places).toFixed(places)}%`;
}

export function sum(list) {
  return list.reduce((total, value) => total + Number(value ?? 0), 0);
}

export function average(list) {
  if (list.length === 0) return 0;
  return sum(list) / list.length;
}

export function ordinal(value) {
  const number = Math.trunc(value);
  const mod100 = number % 100;
  if (mod100 >= 11 && mod100 <= 13) return `${number}th`;
  switch (number % 10) {
    case 1:
      return `${number}st`;
    case 2:
      return `${number}nd`;
    case 3:
      return `${number}rd`;
    default:
      return `${number}th`;
  }
}
