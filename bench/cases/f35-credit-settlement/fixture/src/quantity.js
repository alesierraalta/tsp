export function integer(value, name = 'value') {
  if (!Number.isSafeInteger(value)) {
    throw new TypeError(`${name} must be a safe integer`);
  }
  return value;
}

export function positive(value, name = 'value') {
  integer(value, name);
  if (value <= 0) {
    throw new RangeError(`${name} must be positive`);
  }
  return value;
}

export function nonnegative(value, name = 'value') {
  integer(value, name);
  if (value < 0) {
    throw new RangeError(`${name} must not be negative`);
  }
  return value;
}

export function checkedAdd(left, right) {
  return integer(left + right, 'sum');
}

export function checkedMultiply(left, right) {
  return integer(left * right, 'product');
}

export function clamp(value, minimum, maximum) {
  if (minimum > maximum) {
    throw new RangeError('reversed bounds');
  }
  return Math.min(maximum, Math.max(minimum, value));
}
