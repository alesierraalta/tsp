import test from 'node:test';
import assert from 'node:assert/strict';
import { Router } from '../src/router.js';
import { HttpError } from '../src/http/errors.js';

function context(method, path) {
  return { method, path, headers: {}, query: {}, params: {}, res: { statusCode: 0 } };
}

test('a route matches its pattern and decodes its params', async () => {
  const seen = [];
  const router = new Router();
  router.get('/tenants/:id/members/:userId', async (ctx) => {
    seen.push(ctx.params);
    return 'ok';
  });
  const result = await router.handle(context('GET', '/tenants/tnt_1/members/usr%202'));
  assert.equal(result, 'ok');
  assert.deepEqual(seen[0], { id: 'tnt_1', userId: 'usr 2' });
});

test('a trailing slash matches the same route', async () => {
  const router = new Router();
  router.get('/health', async () => 'healthy');
  assert.equal(await router.handle(context('GET', '/health/')), 'healthy');
});

test('an unknown path and a method mismatch both fall to notFound', async () => {
  const router = new Router();
  router.post('/tenants', async () => 'created');
  await assert.rejects(router.handle(context('GET', '/nowhere')), HttpError);
  await assert.rejects(router.handle(context('GET', '/tenants')), (error) => {
    assert.equal(error.code, 'not_found');
    return true;
  });
});

test('middleware runs in order and can short circuit', async () => {
  const calls = [];
  const router = new Router();
  router.use(async (ctx, next) => {
    calls.push('outer-before');
    await next();
    calls.push('outer-after');
  });
  router.use(async (ctx, next) => {
    calls.push('inner');
    await next();
  });
  router.get('/x', async () => {
    calls.push('handler');
    return 'done';
  });
  await router.handle(context('GET', '/x'));
  assert.deepEqual(calls, ['outer-before', 'inner', 'handler', 'outer-after']);
});

test('a middleware can answer without calling the handler', async () => {
  const router = new Router();
  router.use(async () => 'blocked');
  router.get('/x', async () => {
    throw new Error('the handler should not run');
  });
  assert.equal(await router.handle(context('GET', '/x')), 'blocked');
});

test('HEAD is handled by the GET route', async () => {
  const router = new Router();
  router.get('/health', async () => 'ok');
  assert.equal(await router.handle(context('HEAD', '/health')), 'ok');
});

test('a wildcard route captures the rest of the path', async () => {
  let rest;
  const router = new Router();
  router.get('/files/*', async (ctx) => {
    rest = ctx.params.rest;
  });
  await router.handle(context('GET', '/files/a/b/c.txt'));
  assert.equal(rest, 'a/b/c.txt');
});

test('describe lists every declared route', () => {
  const router = new Router();
  router.get('/a', async () => {});
  router.post('/b/:id', async () => {});
  assert.deepEqual(router.describe(), ['GET /a', 'POST /b/:id']);
});

test('a custom notFound handler is used', async () => {
  const router = new Router({ notFound: () => 'custom' });
  assert.equal(await router.handle(context('GET', '/missing')), 'custom');
});

test('add refuses a missing handler', () => {
  const router = new Router();
  assert.throws(() => router.add('GET', '/x', undefined), TypeError);
  assert.throws(() => router.use('nope'), TypeError);
});
