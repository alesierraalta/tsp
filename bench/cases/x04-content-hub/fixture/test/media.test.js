import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { MediaStore } from '../src/media/store.js';
import { resolveUnder, assertPlainName, extensionOf, mimeOf, isImage, variantName } from '../src/media/paths.js';
import { imageSize, scaleTo, fitsWithin, pngSize, gifSize, nearestPreset, megapixels, crop } from '../src/media/images.js';
import { planVariants, VariantCache, fingerprint, variantKey } from '../src/media/variants.js';
import { normalizeMetadata, mergeMetadata, describeMedia, groupByTag } from '../src/media/metadata.js';
import { UploadSession, planUpload, chunkCount, checksumOf } from '../src/media/upload.js';
import { tempRoots, cleanup } from './helpers.js';

const PNG = Buffer.from(
  '89504e470d0a1a0a0000000d4948445200000060000000600806000000abcdef00',
  'hex',
);

test('plain names are enforced before a path is joined', () => {
  assert.equal(assertPlainName('photo.png'), 'photo.png');
  assert.throws(() => assertPlainName('../secret'), /unsafe path/);
  assert.throws(() => assertPlainName('a/b.png'), /unsafe path/);
  assert.throws(() => assertPlainName(''), /unsafe path/);
  assert.equal(extensionOf('photo.png'), 'png');
  assert.equal(mimeOf('photo.png'), 'image/png');
  assert.equal(isImage('photo.png'), true);
  assert.equal(variantName('photo.png', 'thumb'), 'photo.thumb.png');
});

test('resolveUnder keeps every read inside the root', () => {
  const root = '/tmp/root';
  assert.equal(resolveUnder(root, 'a.png'), '/tmp/root/a.png');
  assert.throws(() => resolveUnder(root, '../a.png'), /unsafe path/);
  assert.throws(() => resolveUnder(root, '/etc/passwd'), /unsafe path/);
});

test('media files round-trip through the store', async () => {
  const roots = await tempRoots();
  try {
    const store = new MediaStore({ root: roots.media });
    const described = await store.put('notes.txt', 'hello');
    assert.equal(described.bytes, 5);
    assert.equal(await store.readText('notes.txt'), 'hello');
    assert.equal((await store.list()).length, 1);
    assert.equal((await store.describe('notes.txt')).type, 'text/plain; charset=utf-8');
    assert.equal(await store.exists('notes.txt'), true);
    assert.equal(await store.totalBytes(), 5);
    assert.equal((await store.largest())[0].name, 'notes.txt');
    assert.deepEqual(await store.dedupe(), []);
    await store.copy('notes.txt', 'copy.txt');
    assert.equal((await store.list()).length, 2);
    await store.rename('copy.txt', 'renamed.txt');
    assert.equal(await store.exists('renamed.txt'), true);
    assert.equal((await store.unused([{ id: 'e1', values: { hero: 'notes.txt' } }])).includes('notes.txt'), false);
    await store.remove('renamed.txt');
    assert.equal(await store.exists('renamed.txt'), false);
  } finally {
    await cleanup(roots);
  }
});

test('image dimensions are read from the header', () => {
  assert.deepEqual(pngSize(PNG), { width: 96, height: 96 });
  assert.deepEqual(imageSize('x.png', PNG), { width: 96, height: 96 });
  assert.equal(imageSize('x.bin', PNG), undefined);
  const gif = Buffer.concat([Buffer.from('GIF89a'), Buffer.from([0x10, 0x00, 0x20, 0x00])]);
  assert.deepEqual(gifSize(gif), { width: 16, height: 32 });
  assert.deepEqual(scaleTo({ width: 800, height: 400 }, { width: 400, height: 400 }), { width: 400, height: 200, scaled: true });
  assert.equal(fitsWithin({ width: 10, height: 10 }, { width: 20, height: 20 }), true);
  assert.equal(nearestPreset({ width: 1600, height: 1200 }), 'hero');
  assert.equal(megapixels({ width: 1000, height: 1000 }), 1);
  assert.deepEqual(crop({ width: 200, height: 100 }, { width: 100, height: 100 }), { width: 100, height: 100, x: 50, y: 0 });
});

test('variant plans follow the presets', () => {
  const plan = planVariants('photo.png', { width: 1000, height: 500 }, ['thumb']);
  assert.equal(plan.length, 1);
  assert.equal(plan[0].name, 'photo.thumb.png');
  assert.equal(plan[0].scaled, true);
  assert.equal(variantKey('photo.png', 'thumb', 'v1'), 'photo:thumb:v1');
  assert.equal(fingerprint(PNG).length, 16);
  assert.equal(planVariants('photo.png', undefined, ['card'])[0].width, 480);
});

test('the variant cache reuses a rendered entry', async () => {
  let renders = 0;
  const cache = new VariantCache({ render: async () => ({ renders: (renders += 1) }) });
  await cache.get('photo.png', 'thumb', 'v1');
  await cache.get('photo.png', 'thumb', 'v1');
  assert.equal(renders, 1);
  assert.equal(cache.stats().hits, 1);
  assert.equal(cache.invalidate('photo.png'), 1);
});

test('metadata defaults and merges', () => {
  assert.equal(normalizeMetadata({}).title, '');
  assert.deepEqual(mergeMetadata({ title: 'A', tags: ['x'] }, { tags: ['y'], title: 'B' }), {
    title: 'B',
    alt: 'A',
    caption: '',
    tags: ['x', 'y'],
  });
  const described = describeMedia('photo.png', PNG, { title: 'Photo' });
  assert.equal(described.dimensions.width, 96);
  assert.deepEqual([...groupByTag([{ name: 'a', tags: ['t'] }]).keys()], ['t']);
});

test('uploads are planned and assembled', async () => {
  assert.deepEqual(planUpload({ name: 'a.txt', declaredBytes: 10 }).problems, []);
  assert.equal(chunkCount(70000), 2);
  assert.equal(checksumOf(Buffer.from('a')).length, 64);
  const roots = await tempRoots();
  try {
    const uploads = new UploadSession({ root: roots.media });
    const session = uploads.begin({ name: 'big.txt', bytes: 6 });
    uploads.append(session.id, 'abc');
    uploads.append(session.id, 'def');
    const done = await uploads.finish(session.id);
    assert.equal(done.bytes, 6);
    assert.equal(uploads.list().length, 1);
    assert.throws(() => uploads.append('upl_missing', 'x'), /does not exist/);
    assert.equal(uploads.abort(session.id), true);
  } finally {
    await cleanup(roots);
  }
});

test('the store refuses oversized files', async () => {
  const roots = await tempRoots();
  try {
    const store = new MediaStore({ root: roots.media, maxBytes: 4 });
    await assert.rejects(() => store.put('big.txt', 'too large'), /at most 4 bytes/);
    await mkdir(roots.media, { recursive: true });
    await writeFile(join(roots.media, 'empty.txt'), '');
    assert.equal((await store.largest())[0].bytes, 0);
  } finally {
    await cleanup(roots);
  }
});
