export class PaymentConsumer {
  constructor(ledger) {
    this.ledger = ledger;
    this.done = new Set();
  }

  async handle(delivery) {
    const { event } = delivery;
    if (this.done.has(event.id)) return;
    const balance = await this.ledger.read(event.account);
    await this.ledger.write(event.account, balance + event.amount);
    this.done.add(event.id);
  }
}
