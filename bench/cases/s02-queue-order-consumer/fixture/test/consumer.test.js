import test from "node:test";
import assert from "node:assert/strict";
import { OrderConsumer } from "../src/consumer.js";
import { OrderStore } from "../src/store.js";

test("an event becomes the order's state", () => {
  const store = new OrderStore();
  const c = new OrderConsumer(store);
  assert.equal(c.handle({ id: "m1", orderId: "o1", version: "1", status: "placed" }), "applied");
  assert.deepEqual(store.get("o1"), { version: "1", status: "placed" });
});

test("a redelivered message is a duplicate", () => {
  const c = new OrderConsumer(new OrderStore());
  const m = { id: "m1", orderId: "o1", version: "1", status: "placed" };
  c.handle(m);
  assert.equal(c.handle(m), "duplicate");
});

test("an older revision arriving late is stale", () => {
  const store = new OrderStore();
  const c = new OrderConsumer(store);
  c.handle({ id: "m2", orderId: "o1", version: "2", status: "paid" });
  assert.equal(c.handle({ id: "m1", orderId: "o1", version: "1", status: "placed" }), "stale");
  assert.equal(store.get("o1").status, "paid");
});
