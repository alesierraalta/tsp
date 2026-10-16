import test from 'node:test';
import assert from 'node:assert/strict';
import { Clock } from '../src/clock.js';
import { Catalog } from '../src/catalog.js';
import { Reservations } from '../src/reservations.js';
import { Schedule } from '../src/schedule.js';
import * as dates from '../src/dates.js';
import * as reports from '../src/reports.js';

test('monthly calendar and previews', () => {
  const s = new Schedule();
  s.add('maintenance', '2024-01-15T09:30:00Z');
  assert.equal(s.next('maintenance'), '2024-02-15T09:30:00.000Z');
  assert.deepEqual(s.preview('maintenance', 2),
    ['2024-03-15T09:30:00.000Z', '2024-04-15T09:30:00.000Z']);
  assert.equal(s.get('maintenance').count, 1);
  s.pause('maintenance');
  assert.throws(() => s.next('maintenance'));
  s.resume('maintenance');
  assert.equal(s.list()[0].active, true);
  const end = new Schedule();
  end.add('close', '2024-01-31T00:00:00Z');
  assert.equal(end.next('close'), '2024-02-29T00:00:00.000Z');
});

test('bookings and capacity', () => {
  const clock = new Clock(0);
  const catalog = new Catalog();
  catalog.register('repair', { capacity: 1, duration: 1000, tags: ['field'] });
  const r = new Reservations(catalog, clock);
  const id = r.hold('repair', 10000, 100);
  assert.equal(r.status(id), 'held');
  assert.throws(() => r.hold('repair', 10000));
  assert.equal(r.confirm(id).status, 'confirmed');
  clock.advance(200);
  assert.equal(r.status(id), 'confirmed');
  assert.equal(reports.agenda(r, 0).length, 1);
  assert.deepEqual(reports.monthly(r, 0), { repair: 1 });
  assert.equal(reports.calendar(r, 0, 2)[1].items.length, 0);
  assert.equal(reports.exportRows(r).length, 1);
  assert.equal(reports.summary(catalog, r).occupied, 1);
  r.cancel(id);
  assert.equal(r.occupancy('repair'), 0);
  const second = r.hold('repair', 10000, 100);
  clock.advance(100);
  assert.equal(r.status(second), 'expired');
  assert.throws(() => r.confirm(second));
  catalog.setEnabled('repair', false);
  assert.throws(() => r.hold('repair', 10000));
  assert.equal(catalog.list('field').length, 1);
  assert.equal(catalog.size, 1);
});

test('clock and calendar helpers', () => {
  const c = new Clock(1000, 50);
  c.adjustWall(-500);
  assert.deepEqual(c.snapshot(), { wall: 500, elapsed: 50 });
  assert.deepEqual(c.measure(() => { c.advance(20); return 'ok'; }), { value: 'ok', duration: 20 });
  assert.equal(dates.daysInMonth(2024, 2), 29);
  assert.equal(dates.compareDays(0, dates.DAY), -1);
  assert.equal(dates.monthStart('2024-03-25'), Date.parse('2024-03-01'));
  assert.throws(() => dates.instant('invalid'));
  assert.throws(() => dates.range(0, 400));
  assert.throws(() => c.advance(-1));
});
