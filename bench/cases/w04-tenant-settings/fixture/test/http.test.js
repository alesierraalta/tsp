import test from 'node:test';
import assert from 'node:assert/strict';
import { Readable } from 'node:stream';
import {
  queryOf,
  readBody,
  readForm,
  readJson,
  requireFields,
  pick,
} from '../src/http/request.js';
import { json, text, html, errorPage, sendError } from '../src/http/response.js';
import { bearerToken, contentType, ifNoneMatch, mergeVary, parseHeaders } from '../src/http/headers.js';
import { HttpError } from '../src/http/errors.js';

function response() {
  const headers = {};
  return {
    statusCode: 0,
    headers,
    body: '',
    setHeader(key, value) {
      headers[key.toLowerCase()] = value;
    },
    end(body) {
      this.body = body ?? '';
    },
    get writableEnded() {
      return this.body !== '';
    },
  };
}

function stream(chunks, headers = {}) {
  const req = Readable.from(chunks.map((chunk) => Buffer.from(chunk)));
  req.headers = headers;
  return req;
}

test('readBody concatenates the request chunks', async () => {
  const body = await readBody(stream(['ab', 'cd'], { 'content-length': '4' }));
  assert.equal(body.toString(), 'abcd');
});

test('readBody refuses a declared length past the cap', async () => {
  await assert.rejects(readBody(stream(['x'], { 'content-length': '5000' }), { limit: 10 }), (error) => {
    assert.equal(error.status, 413);
    assert.equal(error.code, 'body_too_large');
    return true;
  });
});

test('readBody refuses a body that grows past the cap', async () => {
  await assert.rejects(readBody(stream(['1234567890', '1234567890']), { limit: 12 }), (error) => {
    assert.equal(error.code, 'body_too_large');
    return true;
  });
});

test('readJson parses an object body', async () => {
  const body = await readJson(stream(['{"name":"Acme"}'], { 'content-type': 'application/json' }));
  assert.deepEqual(body, { name: 'Acme' });
});

test('readJson refuses other media types, arrays and broken json', async () => {
  await assert.rejects(readJson(stream(['{}'], { 'content-type': 'text/plain' })), (error) => {
    assert.equal(error.code, 'unsupported_media');
    return true;
  });
  await assert.rejects(readJson(stream(['[]'], { 'content-type': 'application/json' })), (error) => {
    assert.equal(error.code, 'bad_request');
    return true;
  });
  await assert.rejects(readJson(stream(['{'], { 'content-type': 'application/json' })), (error) => {
    assert.equal(error.code, 'bad_request');
    return true;
  });
});

test('readForm keeps repeated keys as lists and drops prototype keys', async () => {
  const body = await readForm(
    stream(['a=1&a=2&b=3&__proto__=x'], {
      'content-type': 'application/x-www-form-urlencoded',
    }),
  );
  assert.deepEqual(body.a, ['1', '2']);
  assert.equal(body.b, '3');
  assert.equal(Object.hasOwn(body, '__proto__'), false);
});

test('queryOf reads repeated keys', () => {
  const url = new URL('http://x/tenants?plan=team&tag=a&tag=b&constructor=1');
  const query = queryOf(url);
  assert.equal(query.plan, 'team');
  assert.deepEqual(query.tag, ['a', 'b']);
  assert.equal(Object.hasOwn(query, 'constructor'), false);
});

test('requireFields names what is missing and pick copies declared fields', () => {
  assert.throws(() => requireFields({ name: 'x' }, ['name', 'email']), /email/);
  assert.deepEqual(pick({ name: 'x', plan: 'team', extra: 1 }, ['name']), { name: 'x' });
});

test('parseHeaders lowercases and joins repeated headers', () => {
  const headers = parseHeaders({ 'Content-Type': 'application/json; charset=utf-8', 'X-Thing': ['a', 'b'] });
  assert.equal(headers['content-type'], 'application/json; charset=utf-8');
  assert.equal(contentType(headers), 'application/json');
  assert.equal(headers['x-thing'], 'a, b');
});

test('bearerToken only reads a bearer scheme', () => {
  assert.equal(bearerToken({ authorization: 'Bearer abc' }), 'abc');
  assert.equal(bearerToken({ authorization: 'Basic abc' }), undefined);
  assert.throws(() => bearerToken({ authorization: `Bearer ${'x'.repeat(5000)}` }), HttpError);
});

test('ifNoneMatch and mergeVary normalize tags', () => {
  assert.deepEqual(ifNoneMatch({ 'if-none-match': 'W/"abc", "def"' }), ['abc', 'def']);
  assert.equal(mergeVary('Accept-Encoding', 'Authorization'), 'Accept-Encoding, Authorization');
  assert.equal(mergeVary('', 'Accept'), 'Accept');
});

test('json and text responses carry their content type and body', () => {
  const res = response();
  json(res, 201, { ok: true });
  assert.equal(res.statusCode, 201);
  assert.equal(res.headers['content-type'], 'application/json; charset=utf-8');
  assert.deepEqual(JSON.parse(res.body), { ok: true });

  const plain = response();
  text(plain, 200, 'hello');
  assert.equal(plain.headers['content-type'], 'text/plain; charset=utf-8');
  assert.equal(plain.body, 'hello');
});

test('html escapes the error page', () => {
  const page = errorPage(new HttpError(400, 'bad_request', '<script>alert(1)</script>'));
  assert.match(page, /&lt;script&gt;/);
  assert.equal(page.includes('<script>'), false);
});

test('sendError writes the status and the error code', () => {
  const res = response();
  sendError(res, new HttpError(404, 'not_found', 'nothing here'));
  assert.equal(res.statusCode, 404);
  assert.equal(JSON.parse(res.body).error, 'not_found');
});

test('an unexpected error becomes a 500 without leaking the message', () => {
  const res = response();
  sendError(res, new Error('secret internals'));
  assert.equal(res.statusCode, 500);
  assert.equal(JSON.parse(res.body).message, 'internal error');
});

test('html responses set the html content type', () => {
  const res = response();
  html(res, 200, '<p>ok</p>');
  assert.equal(res.headers['content-type'], 'text/html; charset=utf-8');
  assert.equal(res.body, '<p>ok</p>');
});
