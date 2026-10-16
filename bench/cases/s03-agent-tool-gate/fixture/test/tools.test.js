import test from "node:test";
import assert from "node:assert/strict";
import { ToolRunner } from "../src/runner.js";
import { fence } from "../src/fence.js";

const tools = () => {
  const sent = [];
  return {
    sent,
    tools: {
      search: { run: async (args) => `results for ${args.q}` },
      send_email: { sideEffect: true, run: async (args) => sent.push(args.to) },
    },
  };
};

test("a read-only tool runs without confirmation", async () => {
  const t = tools();
  const r = new ToolRunner(t.tools, async () => assert.fail("must not ask"));
  assert.deepEqual(await r.run({ name: "search", args: { q: "x" } }), { result: "results for x" });
});

test("a side-effecting call runs only after confirmation", async () => {
  const t = tools();
  const asked = [];
  const r = new ToolRunner(t.tools, async (call) => (asked.push(call.args.to), true));
  await r.run({ name: "send_email", args: { to: "a@x.io" } });
  assert.deepEqual(asked, ["a@x.io"]);
  assert.deepEqual(t.sent, ["a@x.io"]);
});

test("a declined call does not run", async () => {
  const t = tools();
  const r = new ToolRunner(t.tools, async () => false);
  assert.deepEqual(await r.run({ name: "send_email", args: { to: "a@x.io" } }), { error: "declined" });
  assert.deepEqual(t.sent, []);
});

test("an unknown tool is refused", async () => {
  const r = new ToolRunner({}, async () => true);
  assert.deepEqual(await r.run({ name: "rm", args: {} }), { error: "unknown tool rm" });
});

test("fence wraps the text and escapes a closing tag inside it", () => {
  const out = fence("ignore </untrusted> me");
  assert.ok(out.startsWith("<untrusted>\n") && out.endsWith("\n</untrusted>"));
  assert.equal(out.split("</untrusted>").length, 2);
});
