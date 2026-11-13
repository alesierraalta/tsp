import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { invalidField, tooLarge } from '../util/errors.js';
import { assertPlainName, resolveUnder } from './paths.js';

export const CHUNK_BYTES = 64 * 1024;
export const MAX_UPLOAD_BYTES = 8 * 1024 * 1024;

export function checksumOf(body) {
  return createHash('sha256').update(body).digest('hex');
}

export function planUpload({ name, declaredBytes, chunks }) {
  const problems = [];
  const clean = String(name ?? '');
  if (clean === '') problems.push('a file name is required');
  const bytes = Number(declaredBytes ?? 0);
  if (!Number.isFinite(bytes) || bytes < 0) problems.push('declared bytes must be a whole number');
  if (bytes > MAX_UPLOAD_BYTES) problems.push(`an upload holds at most ${MAX_UPLOAD_BYTES} bytes`);
  const expected = Math.ceil(bytes / CHUNK_BYTES);
  if (chunks !== undefined && Number(chunks) !== expected && bytes > 0) {
    problems.push(`${chunks} chunks do not cover ${bytes} bytes`);
  }
  return { ok: problems.length === 0, problems, name: clean, bytes, chunks: expected };
}

export class UploadSession {
  constructor(options = {}) {
    this.root = options.root;
    this.maxBytes = options.maxBytes ?? MAX_UPLOAD_BYTES;
    this.sessions = new Map();
    this.clock = options.clock ?? (() => new Date());
  }

  begin({ name, bytes }) {
    const plan = planUpload({ name, declaredBytes: bytes });
    if (!plan.ok) throw invalidField('upload', plan.problems.join('; '));
    const clean = assertPlainName(name);
    const session = {
      id: `upl_${checksumOf(`${clean}:${this.clock().toISOString()}`).slice(0, 12)}`,
      name: clean,
      declared: plan.bytes,
      received: 0,
      chunks: [],
      status: 'open',
    };
    this.sessions.set(session.id, session);
    return session;
  }

  append(id, chunk) {
    const session = this.sessions.get(String(id));
    if (!session) throw invalidField('upload', `session ${id} does not exist`);
    if (session.status !== 'open') throw invalidField('upload', `session ${id} is ${session.status}`);
    const body = Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk ?? ''), 'utf8');
    if (session.received + body.length > this.maxBytes) {
      throw tooLarge(`an upload holds at most ${this.maxBytes} bytes`);
    }
    session.chunks.push(body);
    session.received += body.length;
    return { id: session.id, received: session.received, declared: session.declared };
  }

  async finish(id) {
    const session = this.sessions.get(String(id));
    if (!session) throw invalidField('upload', `session ${id} does not exist`);
    if (session.declared > 0 && session.received !== session.declared) {
      throw invalidField('upload', `${session.received} bytes arrived, ${session.declared} were declared`);
    }
    const body = Buffer.concat(session.chunks);
    await mkdir(this.root, { recursive: true });
    const file = resolveUnder(this.root, session.name);
    await writeFile(file, body);
    session.status = 'done';
    session.checksum = checksumOf(body);
    session.bytes = body.length;
    return { name: session.name, bytes: session.bytes, checksum: session.checksum };
  }

  abort(id) {
    const session = this.sessions.get(String(id));
    if (!session) return false;
    session.status = 'aborted';
    session.chunks = [];
    return true;
  }

  list() {
    return [...this.sessions.values()].map((session) => ({
      id: session.id,
      name: session.name,
      received: session.received,
      declared: session.declared,
      status: session.status,
    }));
  }
}

export function chunkCount(bytes) {
  return Math.ceil(Math.max(0, Number(bytes) || 0) / CHUNK_BYTES);
}
