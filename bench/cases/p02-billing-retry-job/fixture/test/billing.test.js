import test from 'node:test';
import assert from 'node:assert/strict';
import { BillingJob, MemoryLedger, MemoryPayments } from '../src/billing.js';

test('credits completed jobs and records receipts', () => {
  const payments = new MemoryPayments();
  const ledger = new MemoryLedger();
  const worker = new BillingJob(payments, ledger);
  for (const job of [
    { id: 'a', account: 'red', amount: 12 },
    { id: 'b', account: 'red', amount: 8 },
    { id: 'c', account: 'blue', amount: 5 },
  ]) {
    assert.equal(worker.run(job), 'complete');
    assert.equal(ledger.completed(job.id), true);
    assert.equal(worker.run(job), 'complete');
  }
  assert.equal(ledger.balance('red'), 20);
  assert.equal(ledger.balance('blue'), 5);
  assert.equal(ledger.balance('missing'), 0);
  assert.equal(payments.charges.length, 3);
  assert.deepEqual(payments.charges.map(c => c.account), ['red', 'red', 'blue']);
  assert.deepEqual([...ledger.jobs.values()], ['receipt-1', 'receipt-2', 'receipt-3']);
});

test('reports a deferred commit without crediting the account', () => {
  const payments = new MemoryPayments();
  const ledger = new MemoryLedger(() => false);
  const worker = new BillingJob(payments, ledger);
  assert.equal(worker.run({ id: 'a', account: 'red', amount: 7 }), 'retry');
  assert.equal(ledger.balance('red'), 0);
  assert.equal(ledger.completed('a'), false);
});

test('payment adapter replays a keyed receipt', () => {
  const payments = new MemoryPayments();
  const first = payments.charge('red', 800, 'a');
  assert.equal(payments.charge('red', 800, 'a'), first);
  assert.equal(payments.charges.length, 1);
});
