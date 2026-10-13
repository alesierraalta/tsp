import assert from "node:assert/strict";
import { test } from "node:test";
import { Replay } from "../src/replay.js";

test("applies events delivered in sequence order", () => {
  const replay = new Replay();
  replay.apply({ seq: 1, value: "started", at: Date.now() });
  replay.apply({ seq: 2, value: "complete", at: Date.now() });
  assert.equal(replay.state, "complete");
});
