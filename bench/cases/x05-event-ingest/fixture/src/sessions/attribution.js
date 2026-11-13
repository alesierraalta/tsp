export const CHANNELS = ['organic', 'paid', 'referral', 'email', 'direct'];

export function channelOf(event) {
  const context = event?.context ?? {};
  if (context.referrer) {
    const referrer = String(context.referrer).toLowerCase();
    if (referrer.includes('google') || referrer.includes('bing')) return 'organic';
    if (referrer.includes('newsletter') || referrer.includes('email')) return 'email';
    return 'referral';
  }
  if (context.campaign) return 'paid';
  return 'direct';
}

export function attributionOf(events) {
  const first = new Map();
  for (const event of events ?? []) {
    const user = event.context?.user ?? event.id;
    if (!first.has(user)) first.set(user, channelOf(event));
  }
  const counts = {};
  for (const channel of first.values()) counts[channel] = (counts[channel] ?? 0) + 1;
  return counts;
}

export function channelShare(events) {
  const counts = attributionOf(events);
  const total = Object.values(counts).reduce((sum, value) => sum + value, 0);
  return Object.entries(counts)
    .map(([channel, users]) => ({ channel, users, percent: total === 0 ? 0 : Math.round((users / total) * 10000) / 100 }))
    .sort((left, right) => right.users - left.users);
}

export function lastTouch(events) {
  const last = new Map();
  for (const event of events ?? []) {
    const user = event.context?.user ?? event.id;
    last.set(user, channelOf(event));
  }
  return Object.fromEntries(last);
}

export function compareModels(events) {
  const first = attributionOf(events);
  const last = lastTouch(events);
  const changes = Object.keys(first).filter((channel) => (first[channel] ?? 0) !== Object.values(last).filter((value) => value === channel).length);
  return { first, last, divergent: changes };
}

export function normalizeChannel(value) {
  const text = String(value ?? '').toLowerCase();
  return CHANNELS.includes(text) ? text : 'direct';
}
