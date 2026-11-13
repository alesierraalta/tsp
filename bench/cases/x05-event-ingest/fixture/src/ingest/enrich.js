export const DEVICE_KINDS = ['desktop', 'mobile', 'tablet', 'bot'];

export function deviceOf(event) {
  const agent = String(event?.context?.user_agent ?? event?.properties?.user_agent ?? '').toLowerCase();
  if (agent === '') return { kind: 'desktop', bot: false };
  if (/bot|crawler|spider/.test(agent)) return { kind: 'bot', bot: true };
  if (/ipad|tablet/.test(agent)) return { kind: 'tablet', bot: false };
  if (/mobile|iphone|android/.test(agent)) return { kind: 'mobile', bot: false };
  return { kind: 'desktop', bot: false };
}

export function geoOf(event) {
  const country = String(event?.context?.country ?? '').toUpperCase();
  return /^[A-Z]{2}$/.test(country) ? { country, region: event?.context?.region ?? null } : { country: 'ZZ', region: null };
}

export function enrichEvent(event) {
  const device = deviceOf(event);
  const geo = geoOf(event);
  return {
    ...event,
    derived: {
      device: device.kind,
      bot: device.bot,
      country: geo.country,
      region: geo.region,
      campaign: event?.context?.campaign ?? null,
    },
  };
}

export function enrichAll(events) {
  return (events ?? []).map((event) => enrichEvent(event));
}

export function humanShare(events) {
  const enriched = enrichAll(events);
  if (enriched.length === 0) return 0;
  const human = enriched.filter((event) => event.derived.bot === false).length;
  return Math.round((human / enriched.length) * 10000) / 100;
}

export function deviceBreakdown(events) {
  const out = {};
  for (const event of enrichAll(events)) out[event.derived.device] = (out[event.derived.device] ?? 0) + 1;
  return out;
}

export function countryBreakdown(events, limit = 10) {
  const counts = {};
  for (const event of enrichAll(events)) counts[event.derived.country] = (counts[event.derived.country] ?? 0) + 1;
  return Object.entries(counts)
    .map(([country, events]) => ({ country, events }))
    .sort((left, right) => right.events - left.events)
    .slice(0, limit);
}

export function botTraffic(events) {
  const bots = enrichAll(events).filter((event) => event.derived.bot);
  return { events: bots.length, names: [...new Set(bots.map((event) => event.name))] };
}

export function normalizeCampaign(event) {
  const raw = String(event?.context?.campaign ?? '').trim();
  if (raw === '') return null;
  return raw.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').slice(0, 60);
}

export function stripDerived(events) {
  return (events ?? []).map((event) => {
    const copy = { ...event };
    delete copy.derived;
    return copy;
  });
}

export function referrerHost(event) {
  const referrer = String(event?.context?.referrer ?? '').trim();
  if (referrer === '') return null;
  try {
    return new URL(referrer).hostname.toLowerCase();
  } catch {
    return referrer.toLowerCase();
  }
}

export function isSearchReferrer(event) {
  const host = referrerHost(event) ?? '';
  return /(^|\.)(google|bing|duckduckgo|baidu)\.[a-z.]+$/.test(host);
}

export function campaignBreakdown(events) {
  const out = {};
  for (const event of events ?? []) {
    const campaign = normalizeCampaign(event) ?? '(none)';
    out[campaign] = (out[campaign] ?? 0) + 1;
  }
  return out;
}

export function humanEvents(events) {
  return enrichAll(events).filter((event) => event.derived.bot === false);
}

export function botShare(events) {
  const total = (events ?? []).length;
  if (total === 0) return 0;
  return Math.round((botTraffic(events).events / total) * 10000) / 100;
}

export function deviceSummary(events) {
  const breakdown = deviceBreakdown(events);
  return Object.entries(breakdown)
    .map(([device, count]) => ({ device, count }))
    .sort((left, right) => right.count - left.count);
}

export function enrichReport(events) {
  return {
    events: (events ?? []).length,
    human: humanEvents(events).length,
    bots: botShare(events),
    devices: deviceSummary(events),
    countries: countryBreakdown(events, 3),
    campaigns: campaignBreakdown(events),
  };
}
