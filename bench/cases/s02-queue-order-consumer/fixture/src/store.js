export class OrderStore {
  constructor() {
    this.orders = new Map();
  }

  get(orderId) {
    return this.orders.get(orderId);
  }

  apply(orderId, state) {
    this.orders.set(orderId, state);
  }
}
