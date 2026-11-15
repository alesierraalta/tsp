import { countBy, uniqueBy } from '../util/collections.js';
import { percentOf } from '../util/numbers.js';

export function defineFunnel(steps) {
  if (!Array.isArray(steps) || steps.length < 2) throw new TypeError('a funnel needs at least two steps');
  return { steps: steps.map((step) => String(step)) };
}

export function funnelCounts(events, funnel) {
  const order = funnel.steps;
  const reached = new Map();
  for (const event of events ?? []) {
    const index = order.indexOf(event.name);
    if (index < 0) continue;
    const key = event.context?.session ?? event.context?.user ?? event.id;
    const current = reached.get(key) ?? 0;
    if (index === current) reached.set(key, current + 1);
  }
  const counts = order.map((step, index) => ({
    step,
    users: [...reached.values()].filter((value) => value > index).length,
  }));
  return counts;
}

export function funnelRates(counts) {
  const first = counts[0]?.users ?? 0;
  return counts.map((entry) => ({ ...entry, percent: percentOf(entry.users, first) }));
}

export function funnelDropoff(counts) {
  const out = [];
  for (let index = 1; index < counts.length; index += 1) {
    const before = counts[index - 1].users;
    const after = counts[index].users;
    out.push({
      from: counts[index - 1].step,
      to: counts[index].step,
      lost: before - after,
      percent: before === 0 ? 0 : Math.round(((before - after) / before) * 10000) / 100,
    });
  }
  return out;
}

export function describeFunnel(events, funnel) {
  const counts = funnelCounts(events, funnel);
  return {
    steps: counts.length,
    conversion: counts.length === 0 ? 0 : percentOf(counts[counts.length - 1].users, counts[0].users),
    dropoff: funnelDropoff(counts),
  };
}

export function topEvents(events, limit = 5) {
  return countBy(events ?? [], (event) => event.name)
    .values()
    .toArray?.() ?? [...countBy(events ?? [], (event) => event.name).entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((left, right) => right.count - left.count)
    .slice(0, limit);
}

export function uniqueUsers(events) {
  return uniqueBy(events ?? [], (event) => event.context?.user ?? event.id).length;
}
