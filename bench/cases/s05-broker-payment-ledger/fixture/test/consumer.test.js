import test from "node:test";
import assert from "node:assert/strict";
import { PaymentConsumer } from "../src/consumer.js";
import { Ledger } from "../src/ledger.js";

test("a payment adds its amount to the account", async () => {
  const ledger = new Ledger();
  const c = new PaymentConsumer(ledger);
  await c.handle({ deliveryTag: 1, event: { id: "p1", account: "acme", amount: 30 } });
  assert.equal(await ledger.read("acme"), 30);
});

test("payments to one account add up", async () => {
  const ledger = new Ledger();
  const c = new PaymentConsumer(ledger);
  await c.handle({ deliveryTag: 1, event: { id: "p1", account: "acme", amount: 30 } });
  await c.handle({ deliveryTag: 2, event: { id: "p2", account: "acme", amount: 12 } });
  assert.equal(await ledger.read("acme"), 42);
});

test("payments to different accounts stay apart", async () => {
  const ledger = new Ledger();
  const c = new PaymentConsumer(ledger);
  await c.handle({ deliveryTag: 1, event: { id: "p1", account: "acme", amount: 30 } });
  await c.handle({ deliveryTag: 2, event: { id: "p2", account: "zeta", amount: 5 } });
  assert.equal(await ledger.read("acme"), 30);
  assert.equal(await ledger.read("zeta"), 5);
});
