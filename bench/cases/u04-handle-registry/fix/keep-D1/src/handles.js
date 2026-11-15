// Owns the handles members claim: one entry per handle, whatever case or Unicode normalization arrives.
const RESERVED = new Set(['admin', 'billing', 'support', 'café']);
const PATTERN = /^[\p{L}\p{M}\p{N}_.-]{3,20}$/u;
const MAX = 20;

export const ErrInvalid = 'invalid handle';
export const ErrReserved = 'reserved handle';
export const ErrTaken = 'handle taken';

function fold(handle) {
  return String(handle ?? '').trim().normalize('NFC').toUpperCase().toLowerCase();
}

export function canonical(handle) {
  return String(handle ?? '').trim().toLowerCase();
}

function validate(handle) {
  const name = String(handle ?? '').trim();
  if (name.length > MAX || !PATTERN.test(name)) throw new Error(`${ErrInvalid}: ${name}`);
  return name;
}

function byHandle(left, right) {
  if (left.handle === right.handle) return 0;
  return left.handle < right.handle ? -1 : 1;
}

export class Registry {
  constructor({ handles = new Map() } = {}) {
    this.handles = handles;
  }

  isReserved(handle) {
    return RESERVED.has(fold(handle));
  }

  claim(handle, owner = '') {
    const name = validate(handle);
    if (this.isReserved(name)) throw new Error(`${ErrReserved}: ${name}`);
    const key = canonical(name);
    if (this.handles.has(key)) throw new Error(`${ErrTaken}: ${name}`);
    const entry = { handle: name, owner: String(owner ?? ''), since: '2026-01-01' };
    this.handles.set(key, entry);
    return { ...entry };
  }

  lookup(handle) {
    const entry = this.handles.get(canonical(handle));
    return entry === undefined ? undefined : { ...entry };
  }

  rename(current, next) {
    const entry = this.lookup(current);
    if (entry === undefined) return false;
    const name = validate(next);
    if (this.isReserved(name)) throw new Error(`${ErrReserved}: ${name}`);
    const key = canonical(name);
    if (key !== canonical(current) && this.handles.has(key)) throw new Error(`${ErrTaken}: ${name}`);
    this.handles.delete(canonical(current));
    this.handles.set(key, { ...entry, handle: name });
    return true;
  }

  release(handle) {
    return this.handles.delete(canonical(handle));
  }

  list() {
    return [...this.handles.values()].map((entry) => ({ ...entry })).sort(byHandle);
  }

  render(handle) {
    const entry = this.lookup(handle);
    return entry === undefined ? '' : `@${entry.handle}`;
  }
}
