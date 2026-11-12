import test from 'node:test';
import assert from 'node:assert/strict';
import { Allowlist, EgressBlocked, describe, hostOf, isInternal, sameOrigin } from '../src/egress.js';

test('a listed host is admitted', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com'] });
  const url = guard.check('https://example-cdn.com/track.gif');
  assert.equal(url.hostname, 'example-cdn.com');
  assert.equal(url.protocol, 'https:');
});

test('a subdomain of a listed host is admitted', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com'] });
  assert.equal(guard.check('https://cdn.example-cdn.com/track.gif').hostname, 'cdn.example-cdn.com');
});

test('a host that is not listed is refused', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com'] });
  assert.throws(() => guard.check('https://other.example.org/track.gif'), EgressBlocked);
});

test('an empty allowlist admits any public host', () => {
  const guard = new Allowlist();
  assert.equal(guard.check('https://partner.example.org/hook').hostname, 'partner.example.org');
});

test('http is refused unless it is admitted', () => {
  assert.throws(() => new Allowlist().check('http://example-cdn.com/a.png'), EgressBlocked);
  assert.equal(new Allowlist({ allowHttp: true }).check('http://example-cdn.com/a.png').protocol, 'http:');
});

test('a relative url is refused', () => {
  assert.throws(() => new Allowlist().check('/a.png'), EgressBlocked);
  assert.throws(() => new Allowlist().check(''), EgressBlocked);
});

test('internal addresses are refused whatever the allowlist says', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com'] });
  for (const target of [
    'https://127.0.0.1/admin',
    'https://10.1.2.3/admin',
    'https://192.168.0.5/admin',
    'https://169.254.169.254/latest/meta-data',
    'https://172.16.0.1/admin',
    'https://localhost:8080/admin',
    'https://[::1]/admin',
  ]) {
    assert.throws(() => guard.check(target), EgressBlocked, target);
  }
});

test('isInternal knows the internal hosts', () => {
  assert.equal(isInternal('127.0.0.1'), true);
  assert.equal(isInternal('localhost'), true);
  assert.equal(isInternal('10.0.0.1'), true);
  assert.equal(isInternal('example-cdn.com'), false);
});

test('hostOf returns the host the request reaches', () => {
  assert.equal(hostOf('https://cdn.example-cdn.com:8443/a.png'), 'cdn.example-cdn.com');
  assert.throws(() => hostOf('a.png'), EgressBlocked);
});

test('describe writes host and port', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com'] });
  assert.equal(describe(guard.check('https://example-cdn.com/a.png')), 'example-cdn.com');
  assert.equal(describe(guard.check('https://example-cdn.com:8443/a.png')), 'example-cdn.com:8443');
});

test('sameOrigin compares scheme, host and port', () => {
  assert.equal(sameOrigin('https://example-cdn.com/a', 'https://example-cdn.com:443/b'), true);
  assert.equal(sameOrigin('https://example-cdn.com/a', 'https://cdn.example-cdn.com/a'), false);
  assert.equal(sameOrigin('https://example-cdn.com/a', 'https://example-cdn.com:8443/a'), false);
});

test('a trailing dot in the allowlist entry is ignored', () => {
  const guard = new Allowlist({ hosts: ['example-cdn.com.'] });
  assert.equal(guard.check('https://example-cdn.com/a.png').hostname, 'example-cdn.com');
});
