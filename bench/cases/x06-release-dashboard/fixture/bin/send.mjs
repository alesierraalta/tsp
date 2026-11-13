#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';
import { WebhookSender, recordingTransport } from '../src/webhooks/sender.js';
import { buildPayload } from '../src/webhooks/payloads.js';
import { assertTarget, describeDecision } from '../src/webhooks/targets.js';
import { signDelivery, verifySignature } from '../src/webhooks/signature.js';

function parse(argv) {
  const out = { positional: [], flags: {} };
  for (const arg of argv) {
    if (arg.startsWith('--')) {
      const [name, value] = arg.slice(2).split('=');
      out.flags[name] = value === undefined ? 'true' : value;
      continue;
    }
    out.positional.push(arg);
  }
  return out;
}

function fail(message, code = 2) {
  process.stderr.write(`send: ${message}\n`);
  process.exit(code);
}

const args = parse(process.argv.slice(2));
const url = args.positional[0];
if (!url) fail('a target url is required');

const event = String(args.flags.event ?? 'release.created');
const secret = String(args.flags.secret ?? 'development-webhook-secret');
const allowed = String(args.flags['allowed-hosts'] ?? '')
  .split(',')
  .map((host) => host.trim())
  .filter(Boolean);

if (args.flags.check === 'true') {
  const decision = describeDecision(url, { allowed });
  process.stdout.write(`${JSON.stringify(decision, null, 2)}\n`);
  process.exit(decision.allowed ? 0 : 1);
}

let payload;
if (args.flags.file) {
  const path = resolve(isAbsolute(args.flags.file) ? args.flags.file : `${process.cwd()}/${args.flags.file}`);
  const text = await readFile(path, 'utf8').catch(() => fail(`${args.flags.file} cannot be read`));
  try {
    payload = JSON.parse(text);
  } catch (error) {
    fail(`${args.flags.file} is not valid JSON: ${error.message}`);
  }
} else {
  payload = buildPayload(event, { service: args.flags.service ?? 'checkout', version: args.flags.version ?? '1.0.0' });
}

const body = JSON.stringify(payload);
if (args.flags.dry === 'true') {
  const target = assertTarget(url, { allowed });
  process.stdout.write(`${JSON.stringify({ target: targetSummaryOf(target), signature: signDelivery(body, secret) }, null, 2)}\n`);
  process.exit(0);
}

const recorder = recordingTransport();
const sender = new WebhookSender({ transport: recorder.transport, allowed });
const result = await sender.send({ url, payload, secret }).catch((error) => fail(error.message, 1));
const verified = verifySignature(body, result.headers['x-dashboard-signature'], secret);

process.stdout.write(`${JSON.stringify({ posted: recorder.posted.length, host: result.host, status: result.status, signature_ok: verified.ok }, null, 2)}\n`);
if (!verified.ok) process.exitCode = 1;

function targetSummaryOf(target) {
  return { host: target.host, path: target.path };
}
