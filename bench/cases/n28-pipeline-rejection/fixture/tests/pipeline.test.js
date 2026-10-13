import test from "node:test";
import assert from "node:assert/strict";
import { runPipeline } from "../src/pipeline.js";

test("returns resolved item results in input order", async () => {
  const results = await runPipeline(
    [{ id: "slow", delay: 8 }, { id: "fast", delay: 0 }],
    async (item) => {
      await new Promise((resolve) => setTimeout(resolve, item.delay));
      return item.id.toUpperCase();
    },
  );
  assert.deepEqual(results, [
    { id: "slow", value: "SLOW" },
    { id: "fast", value: "FAST" },
  ]);
});

test("logs and skips an item that exceeds its timeout", async () => {
  const logged = [];
  const results = await runPipeline(
    [{ id: "late" }],
    () => new Promise((resolve) => setTimeout(() => resolve("done"), 15)),
    { timeoutMs: 2, onTimeout: (item) => logged.push(item.id) },
  );
  assert.deepEqual(results, []);
  assert.deepEqual(logged, ["late"]);
});
