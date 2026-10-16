const weights = { adult: 84, child: 35 };
const labels = { Mr: 'adult', Ms: 'adult', Mstr: 'child', Miss: 'child' };

function unpack(value) {
  if (Array.isArray(value)) {
    const [name, title, category, destination, weight] = value;
    return { name, title, category, destination, weight };
  }
  return { ...value };
}

function measure(parts) {
  return weights[parts[1] || labels[parts[0]]];
}

export class Register {
  constructor({ store = new Map() } = {}) {
    this.store = store;
  }

  add(id, profile) {
    this.store.set(id, { ...profile, weight: weights[profile.category] });
  }

  get(id) {
    const value = this.store.get(id);
    return value === undefined ? undefined : unpack(value);
  }

  amend(id, patch) {
    const value = this.store.get(id);
    if (value === undefined) return false;
    const next = { ...unpack(value), ...patch };
    if (Object.hasOwn(patch, 'title')) next.weight = measure([next.title, next.category]);
    this.store.set(id, next);
    return true;
  }
}
