import test from 'node:test';
import assert from 'node:assert/strict';
import {
  hostOf,
  hostPort,
  isAbsolute,
  joinPath,
  originOf,
  parseUrl,
  pathOf,
  queryPairs,
  sameOrigin,
  withoutCredentials,
} from '../src/util/url.js';
import { EgressRules, internalRules, isAllowedHost, normaliseHost } from '../src/egress/rules.js';
import {
  assertAllowlisted,
  assertPublicHost,
  assertScheme,
  assertTarget,
  describeTarget,
  isInternalHost,
  planFor,
  probeFor,
} from '../src/egress/url.js';
import { EgressError } from '../src/util/errors.js';

test('url helpers read the parts of an absolute url', () => {
  assert.equal(originOf('https://Example.com:8443/a?b=1'), 'https://example.com:8443');
  assert.equal(hostOf('https://cdn.example.com/a'), 'cdn.example.com');
  assert.equal(pathOf('https://example.com/a/b?q=1'), '/a/b?q=1');
  assert.deepEqual(queryPairs('https://example.com/?a=1&b=2'), [['a', '1'], ['b', '2']]);
  assert.equal(withoutCredentials('https://user:pass@example.com/a'), 'https://example.com/a');
  assert.equal(sameOrigin('https://example.com/a', 'https://example.com:443/b'), true);
  assert.equal(sameOrigin('https://example.com/a', 'https://cdn.example.com/a'), false);
  assert.equal(hostPort('https://example.com:8443/x'), 'example.com:8443');
  assert.equal(joinPath('https://example.com/base', 'a', 'b'), 'https://example.com/base/a/b');
  assert.equal(isAbsolute('https://example.com'), true);
  assert.equal(isAbsolute('/relative'), false);
});

test('parseUrl refuses what is not an http url', () => {
  assert.throws(() => parseUrl(''), /absolute URL/);
  assert.throws(() => parseUrl('not a url'), /absolute URL/);
  assert.throws(() => parseUrl('ftp://example.com/x'), /unsupported URL scheme/);
});

test('host normalisation strips case and a trailing dot', () => {
  assert.equal(normaliseHost(' Example.COM. '), 'example.com');
  assert.equal(normaliseHost(undefined), '');
});

test('the allowlist admits the host and its subdomains only', () => {
  const rules = new EgressRules({ allowlist: ['example.com'] });
  assert.equal(rules.allowsHost('example.com'), true);
  assert.equal(rules.allowsHost('cdn.example.com'), true);
  assert.equal(rules.allowsHost('notexample.com'), false);
  assert.equal(rules.allowsHost('example.com.evil.test'), false);
  assert.equal(isAllowedHost(new EgressRules(), 'anything.test'), true);
  assert.equal(rules.describe(), 'example.com');
});

test('schemes follow allowInsecure', () => {
  assert.equal(new EgressRules().allowsScheme('https:'), true);
  assert.equal(new EgressRules().allowsScheme('http:'), false);
  assert.equal(new EgressRules({ allowInsecure: true }).allowsScheme('http:'), true);
});

test('internal addresses are refused by hostname', () => {
  const rules = new EgressRules();
  for (const host of ['127.0.0.1', '10.1.2.3', '192.168.0.5', '169.254.169.254', '172.16.0.1', 'localhost', '[::1]']) {
    assert.equal(isInternalHost(host, rules), true, host);
  }
  assert.equal(isInternalHost('example.com', rules), false);
  assert.equal(probeFor('127.0.0.1'), 'http://127.0.0.1/');
  assert.equal(new EgressRules({ allowInternal: true }).blocksInternal('http://127.0.0.1/'), false);
  assert.ok(internalRules().length > 0);
});

test('assertTarget applies scheme, allowlist and internal rules to the parsed host', () => {
  const rules = new EgressRules({ allowlist: ['example.com'] });
  assert.equal(assertTarget('https://cdn.example.com/hook', rules).hostname, 'cdn.example.com');
  assert.throws(() => assertTarget('http://cdn.example.com/hook', rules), (error) => {
    assert.ok(error instanceof EgressError);
    assert.match(error.message, /scheme http:/);
    return true;
  });
  assert.throws(() => assertTarget('https://other.test/hook', rules), /not on the allowlist/);
  const open = new EgressRules({ allowInsecure: true });
  assert.throws(() => assertTarget('https://127.0.0.1/admin', open), /internal address/);
  assert.throws(() => assertTarget('https://localhost:8080/admin', open), /internal address/);
  assert.throws(() => assertTarget('nope', open), /egress blocked/);
  assert.throws(() => assertPublicHost('10.0.0.1', open), /internal address/);
});

test('the guard helpers run in the order the contract states', () => {
  const rules = new EgressRules({ allowInsecure: true });
  const url = parseUrl('https://example.com/a');
  assert.equal(assertScheme(url, rules).hostname, 'example.com');
  assert.equal(assertAllowlisted(url, rules).hostname, 'example.com');
  assert.equal(describeTarget('https://example.com:8443/a', rules), 'example.com:8443');
  assert.equal(planFor('https://example.com/a', rules).path, '/a');
  assert.throws(() => assertAllowlisted(parseUrl('https://other.test/a'), new EgressRules({ allowlist: ['example.com'] })), /allowlist/);
});
