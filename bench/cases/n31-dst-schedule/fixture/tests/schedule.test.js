import assert from "node:assert/strict";
import test from "node:test";
import { nextRunAt, withJitter } from "../src/schedule.js";

test("selects a future scheduled time on the current local date", () => {
  const now = new Date(2024, 0, 8, 8, 0);
  const next = nextRunAt({ hour: 8, minute: 15, now: () => now });

  assert.equal(next.getTime(), new Date(2024, 0, 8, 8, 15).getTime());
});

test("selects the next local day after today's scheduled time", () => {
  const now = new Date(2024, 0, 8, 8, 30);
  const next = nextRunAt({ hour: 8, minute: 15, now: () => now });

  assert.equal(next.getTime(), new Date(2024, 0, 9, 8, 15).getTime());
});

test("jitter stays within the documented five-minute range", () => {
  const runAt = new Date(2024, 0, 8, 8, 15);

  assert.equal(withJitter(runAt, () => 0).getTime(), runAt.getTime() - 5 * 60_000);
  assert.equal(withJitter(runAt, () => 0.999999).getTime(), runAt.getTime() + 5 * 60_000);
});
