import { diffSeconds, toInstant } from '../util/time.js';

export const JOURNEY_GAP_SECONDS = 1800;

export function journeysOf(events, gapSeconds = JOURNEY_GAP_SECONDS) {
  const sorted = [...(events ?? [])].sort((left, right) => (left.at < right.at ? -1 : 1));
  const out = [];
  let current;
  for (const event of sorted) {
    if (!current || diffSeconds(current.last_at, event.at) > gapSeconds) {
      current = {
        user: event.context?.user ?? 'anonymous',
        started_at: event.at,
        last_at: event.at,
        steps: [{ name: event.name, at: event.at }],
      };
      out.push(current);
      continue;
    }
    current.last_at = event.at;
    current.steps.push({ name: event.name, at: event.at });
  }
  return out;
}

export function journeyLengths(events) {
  return journeysOf(events).map((journey) => journey.steps.length);
}

export function commonPaths(events, limit = 5) {
  const counts = new Map();
  for (const journey of journeysOf(events)) {
    const path = journey.steps.map((step) => step.name).join(' > ');
    counts.set(path, (counts.get(path) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([path, count]) => ({ path, count }))
    .sort((left, right) => right.count - left.count)
    .slice(0, limit);
}

export function dropOffSteps(events) {
  const journeys = journeysOf(events);
  const at = new Map();
  for (const journey of journeys) {
    const last = journey.steps[journey.steps.length - 1]?.name ?? '(none)';
    at.set(last, (at.get(last) ?? 0) + 1);
  }
  return [...at.entries()].map(([step, exits]) => ({ step, exits })).sort((left, right) => right.exits - left.exits);
}

export function journeyDuration(journey) {
  return diffSeconds(journey.started_at, journey.last_at);
}

export function averageJourney(events) {
  const journeys = journeysOf(events);
  if (journeys.length === 0) return { journeys: 0, steps: 0, seconds: 0 };
  return {
    journeys: journeys.length,
    steps: Math.round((journeys.reduce((total, journey) => total + journey.steps.length, 0) / journeys.length) * 100) / 100,
    seconds: Math.round(journeys.reduce((total, journey) => total + journeyDuration(journey), 0) / journeys.length),
  };
}

export function journeyStarts(events) {
  return journeysOf(events).map((journey) => toInstant(journey.started_at)?.toISOString());
}

export function stepCounts(events) {
  const counts = new Map();
  for (const journey of journeysOf(events)) {
    for (const step of journey.steps) counts.set(step.name, (counts.get(step.name) ?? 0) + 1);
  }
  return [...counts.entries()].map(([step, visits]) => ({ step, visits })).sort((left, right) => right.visits - left.visits);
}

export function transitions(events) {
  const counts = new Map();
  for (const journey of journeysOf(events)) {
    for (let index = 0; index + 1 < journey.steps.length; index += 1) {
      const label = `${journey.steps[index].name} > ${journey.steps[index + 1].name}`;
      counts.set(label, (counts.get(label) ?? 0) + 1);
    }
  }
  return counts;
}

export function commonTransitions(events, limit = 5) {
  return [...transitions(events).entries()]
    .map(([label, count]) => ({ transition: label, count }))
    .sort((left, right) => right.count - left.count)
    .slice(0, limit);
}

export function revisitedSteps(events) {
  const out = [];
  for (const journey of journeysOf(events)) {
    const seen = new Set();
    for (const step of journey.steps) {
      if (seen.has(step.name)) out.push({ user: journey.user, step: step.name });
      seen.add(step.name);
    }
  }
  return out;
}

export function journeyReport(events) {
  const journeys = journeysOf(events);
  const average = averageJourney(events);
  const longest = journeys.reduce((best, journey) => (best === undefined || journey.steps.length > best.steps.length ? journey : best), undefined);
  return {
    journeys: journeys.length,
    steps: average.steps,
    seconds: average.seconds,
    longest: longest === undefined ? null : { user: longest.user, steps: longest.steps.length },
    top: commonPaths(events, 3),
  };
}
