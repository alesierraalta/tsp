import test from 'node:test';
import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';
import {
  decodeToken,
  refreshToken,
  signToken,
  tokenFingerprint,
  ttlOf,
  verifyToken,
} from '../src/auth/tokens.js';
import {
  assertCan,
  assertPlatform,
  can,
  describePrincipal,
  normalizePrincipal,
  principalFromHeaders,
  principalFromToken,
  rightsOf,
} from '../src/auth/principal.js';
import { HttpError } from '../src/http/errors.js';

const SECRET = 'unit-secret';

test('a signed token verifies and carries its claims', () => {
  const token = signToken({ sub: 'usr_1', tenant: 'tnt_1', roles: ['admin'] }, SECRET);
  const claims = verifyToken(token, SECRET);
  assert.equal(claims.sub, 'usr_1');
  assert.equal(claims.tenant, 'tnt_1');
  assert.deepEqual(claims.roles, ['admin']);
});

test('a token signed with another secret is refused', () => {
  const token = signToken({ sub: 'usr_1' }, SECRET);
  assert.throws(() => verifyToken(token, 'other-secret'), (error) => {
    assert.equal(error.status, 401);
    return true;
  });
});

test('a tampered payload is refused', () => {
  const token = signToken({ sub: 'usr_1', roles: ['viewer'] }, SECRET);
  const [body, signature] = token.split('.');
  const forged = Buffer.from(JSON.stringify({ sub: 'usr_1', roles: ['owner'], exp: 9999999999 }))
    .toString('base64url');
  assert.throws(() => verifyToken(`${forged}.${signature}`, SECRET), HttpError);
  assert.equal(decodeToken(`${body}.${signature}`).payload.roles[0], 'viewer');
});

test('an expired token is refused and a fresh one is accepted', () => {
  const now = Date.UTC(2026, 0, 1, 12, 0, 0);
  const token = signToken({ sub: 'usr_1' }, SECRET, { now, ttlSeconds: 60 });
  assert.equal(verifyToken(token, SECRET, { now: now + 30_000 }).sub, 'usr_1');
  assert.throws(() => verifyToken(token, SECRET, { now: now + 600_000 }), /expired/);
});

test('a token with no expiry is refused', () => {
  const body = Buffer.from(JSON.stringify({ sub: 'usr_1' })).toString('base64url');
  const signature = createHmac('sha256', SECRET).update(body).digest('base64url');
  assert.throws(() => verifyToken(`${body}.${signature}`, SECRET), /expiry/);
});

test('refresh keeps the claims and moves the expiry', () => {
  const now = Date.UTC(2026, 0, 1);
  const token = signToken({ sub: 'usr_1', roles: ['member'] }, SECRET, { now, ttlSeconds: 60 });
  const next = refreshToken(token, SECRET, { now: now + 30_000, ttlSeconds: 600 });
  const claims = verifyToken(next, SECRET, { now: now + 240_000 });
  assert.equal(claims.sub, 'usr_1');
  assert.deepEqual(claims.roles, ['member']);
  assert.ok(ttlOf(next) > ttlOf(token));
});

test('a malformed token is refused', () => {
  assert.throws(() => decodeToken('nodots'), HttpError);
  assert.throws(() => decodeToken('a.b.c'), HttpError);
  assert.throws(() => decodeToken('!!!.sig'), HttpError);
});

test('a token fingerprint is stable and short', () => {
  assert.equal(tokenFingerprint('abc'), tokenFingerprint('abc'));
  assert.notEqual(tokenFingerprint('abc'), tokenFingerprint('abd'));
  assert.equal(tokenFingerprint('abc').length, 16);
});

test('rights accumulate over roles', () => {
  const principal = normalizePrincipal({ sub: 'u', tenant: 't', roles: ['member', 'admin'] });
  const rights = rightsOf(principal);
  assert.ok(rights.has('write'));
  assert.ok(rights.has('admin'));
  assert.equal(can(principal, 'billing', 't'), false);
  assert.equal(can(principal, 'read', 'other'), false);
  assert.equal(can(principal, 'read', 't'), true);
});

test('a platform principal reaches every tenant', () => {
  const principal = normalizePrincipal({ sub: 'ops', tenant: '', roles: ['platform'] });
  assert.equal(can(principal, 'admin', 'anything'), true);
  assert.equal(assertCan(principal, 'billing', 'tnt_9').id, 'ops');
  assert.equal(assertPlatform(principal).id, 'ops');
});

test('assertCan refuses anonymous and cross tenant callers', () => {
  assert.throws(() => assertCan(undefined, 'read', 't'), (error) => error.status === 401);
  const principal = normalizePrincipal({ sub: 'u', tenant: 'a', roles: ['viewer'] });
  assert.throws(() => assertCan(principal, 'write', 'a'), (error) => error.status === 403);
  assert.throws(() => assertPlatform(principal), /platform/);
});

test('principalFromToken normalizes unknown roles away', () => {
  const token = signToken({ sub: 'u', tenant: 't', roles: ['member', 'wizard'] }, SECRET);
  const principal = principalFromToken(token, SECRET);
  assert.deepEqual(principal.roles, ['member']);
  assert.equal(describePrincipal(principal), 'u (member)');
});

test('principalFromHeaders reads the bearer token', () => {
  const token = signToken({ sub: 'u', tenant: 't', roles: ['member'] }, SECRET);
  const principal = principalFromHeaders({ authorization: `Bearer ${token}` }, SECRET);
  assert.equal(principal.tenantId, 't');
  assert.throws(() => principalFromHeaders({}, SECRET), HttpError);
});

test('role names outside the table never grant a right', () => {
  const principal = normalizePrincipal({ sub: 'u', roles: ['superuser'] });
  assert.deepEqual(principal.roles, []);
  assert.equal(rightsOf(principal).size, 0);
});
