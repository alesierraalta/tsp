export class PaymentConsumer {
  constructor(ledger) {
    this.ledger = ledger;
    this.done = new Set();
    this.tails = new Map();
  }

  async handle(delivery) {
    const { deliveryTag, event } = delivery;
    const prev = this.tails.get(event.account) ?? Promise.resolve();
    const run = prev.then(async () => {
      if (this.done.has(deliveryTag)) return;
      const balance = await this.ledger.read(event.account);
      await this.ledger.write(event.account, balance + event.amount);
      this.done.add(deliveryTag);
    });
    this.tails.set(event.account, run.catch(() => {}));
    return run;
  }
}
