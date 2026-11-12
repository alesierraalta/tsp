import test from 'node:test';
import assert from 'node:assert/strict';
import {
  attr,
  escapeText,
  money,
  renderJsonLd,
  renderRich,
  renderRows,
  renderSummary,
  total,
  truncate,
} from '../src/report.js';

const order = {
  id: 'A-1001',
  title: 'Spring restock',
  customer: 'Avery Hill',
  note: '<b>Leave at the door</b>',
  receiptUrl: 'https://billing.example.com/receipt/A-1001',
  items: [
    { label: 'Linen shirt', quantity: 2, cents: 4500 },
    { label: 'Wool socks', quantity: 1, cents: 1200 },
  ],
};

test('element text escapes the markup characters', () => {
  assert.equal(escapeText('<script>'), '&lt;script&gt;');
  assert.equal(escapeText('a & b'), 'a &amp; b');
  assert.equal(escapeText(undefined), '');
});

test('an attribute value is rendered between double quotes', () => {
  assert.equal(attr('data-customer', 'Avery Hill'), 'data-customer="Avery Hill"');
  assert.equal(attr('href', 'https://billing.example.com/r'), 'href="https://billing.example.com/r"');
});

test('an attribute name outside the pattern is refused', () => {
  assert.throws(() => attr('data customer', 'x'), TypeError);
  assert.throws(() => attr('', 'x'), TypeError);
});

test('the note keeps its inline formatting', () => {
  assert.equal(renderRich('<b>Leave at the door</b>'), '<b>Leave at the door</b>');
  assert.equal(renderRich('<em>asap</em>'), '<em>asap</em>');
});

test('the note falls back to text when it does not start with an allowed tag', () => {
  assert.equal(renderRich('<table>rows</table>'), '&lt;table&gt;rows&lt;/table&gt;');
  assert.equal(renderRich('plain note'), 'plain note');
  assert.equal(renderRich(undefined), '');
});

test('money formats integer cents', () => {
  assert.equal(money(4500), '45.00 EUR');
  assert.equal(money(5), '0.05 EUR');
  assert.equal(money(-120), '-1.20 EUR');
  assert.throws(() => money(12.5), TypeError);
});

test('truncate only marks text it shortened', () => {
  assert.equal(truncate('Linen shirt', 11), 'Linen shirt');
  assert.equal(truncate('Linen shirt', 6), 'Linen…');
  assert.throws(() => truncate('x', 0), TypeError);
});

test('rows carry the label, the quantity and the line price', () => {
  assert.equal(
    renderRows([{ label: 'Linen shirt', quantity: 2, cents: 4500 }]),
    '<tr><td>Linen shirt</td><td>2</td><td>45.00 EUR</td></tr>',
  );
});

test('the total multiplies each line by its quantity', () => {
  assert.equal(total(order.items), 10200);
});

test('the JSON-LD block carries the order and escapes its markup', () => {
  const block = renderJsonLd({ ...order, title: 'A </script> B' });
  assert.match(block, /^<script type="application\/ld\+json">/);
  assert.match(block, /A \\u003c\/script> B/);
  assert.doesNotMatch(block.slice('<script type="application/ld+json">'.length), /<\/script>.*<\/script>/);
});

test('the summary page renders the order', () => {
  const page = renderSummary(order);
  assert.match(page, /<title>Spring restock<\/title>/);
  assert.match(page, /<h1>Spring restock<\/h1>/);
  assert.match(page, /<p class="customer" data-customer="Avery Hill">Avery Hill<\/p>/);
  assert.match(page, /<p class="note"><b>Leave at the door<\/b><\/p>/);
  assert.match(page, /<td>Linen shirt<\/td><td>2<\/td><td>45.00 EUR<\/td>/);
  assert.match(page, /<p class="total">102.00 EUR<\/p>/);
  assert.match(page, /<a href="https:\/\/billing.example.com\/receipt\/A-1001">receipt<\/a>/);
});
