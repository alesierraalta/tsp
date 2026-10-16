import test from "node:test";
import assert from "node:assert/strict";
import { render } from "../src/render.js";

test("bold and line breaks", () => {
  assert.equal(render("**hi**\nthere"), "<strong>hi</strong><br>there");
});

test("html in the reply is shown as text", () => {
  assert.equal(render("<script>alert(1)</script>"), "&lt;script&gt;alert(1)&lt;/script&gt;");
});

test("a link renders as an anchor", () => {
  assert.equal(render("[docs](https://example.com/docs)"), '<a href="https://example.com/docs" rel="noopener">docs</a>');
});

test("an image from the app renders", () => {
  assert.equal(render("![chart](/files/chart.png)"), '<img alt="chart" src="/files/chart.png">');
});
