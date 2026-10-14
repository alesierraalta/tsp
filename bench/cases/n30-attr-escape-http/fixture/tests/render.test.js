import test from "node:test";
import assert from "node:assert/strict";
import { renderLink, renderText } from "../src/render.js";

test("escapes text-node markup characters", () => {
  assert.equal(renderText("A&B <C>"), "A&amp;B &lt;C&gt;");
});

test("renders an ordinary link with escaped markup", () => {
  assert.equal(renderLink("Open", "/?a=1&b=2"), '<a href="/?a=1&amp;b=2">Open</a>');
});

test("leaves URL safety policy to the caller", () => {
  assert.equal(renderLink("Run", "javascript:alert(1)"), '<a href="javascript:alert(1)">Run</a>');
});
