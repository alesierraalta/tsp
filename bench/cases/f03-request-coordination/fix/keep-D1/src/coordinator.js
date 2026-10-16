export class SeatPool {
  constructor(capacity, authorize) {
    this.available = capacity;
    this.authorize = authorize;
  }

  async reserve(customer) {
    if (this.available === 0) return false;
    await this.authorize(customer);
    this.available -= 1;
    return true;
  }
}

export class ProfileCache {
  constructor(fetchProfile) {
    this.fetchProfile = fetchProfile;
    this.values = new Map();
  }

  async get(key) {
    if (this.values.has(key)) return this.values.get(key);
    const pending = Promise.resolve().then(() => this.fetchProfile(key));
    this.values.set(key, pending);
    try {
      const value = await pending;
      this.values.set(key, value);
      return value;
    } catch (error) {
      this.values.delete(key);
      throw error;
    }
  }
}
