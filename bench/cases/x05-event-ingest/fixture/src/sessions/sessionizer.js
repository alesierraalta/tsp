import { bucketOf, diffSeconds, toInstant } from '../util/time.js';

export const SESSION_GAP_SECONDS = 1800;

export class Sessionizer {
  constructor(options = {}) {
    this.gapSeconds = options.gapSeconds ?? SESSION_GAP_SECONDS;
    this.sessions = new Map();
  }

  keyOf(tenant, user) {
    return `${tenant}|${user ?? 'anonymous'}`;
  }

  assign(tenant, event) {
    const user = event.context?.user ?? null;
    const key = this.keyOf(tenant, user);
    const current = this.sessions.get(key);
    const at = toInstant(event.at) ?? new Date();
    if (current && diffSeconds(current.last_at, at) <= this.gapSeconds) {
      current.last_at = at.toISOString();
      current.events.push(event.id);
      return current;
    }
    const session = {
      id: `ses_${key.split('|')[1] ?? 'anon'}_${Math.floor(at.getTime() / 1000)}`,
      tenant,
      user,
      started_at: at.toISOString(),
      last_at: at.toISOString(),
      events: [event.id],
    };
    this.sessions.set(key, session);
    return session;
  }

  build(tenant, events) {
    const sorted = [...events].sort((left, right) => (left.at < right.at ? -1 : 1));
    return sorted.map((event) => this.assign(tenant, event)).reduce((out, session) => {
      const existing = out.find((entry) => entry.id === session.id);
      if (!existing) out.push({ ...session, events: [...session.events] });
      else existing.events = [...new Set([...existing.events, ...session.events])];
      return out;
    }, []);
  }

  list(tenant) {
    return [...this.sessions.values()].filter((session) => session.tenant === tenant);
  }

  duration(session) {
    return diffSeconds(session.started_at, session.last_at);
  }

  pageViews(tenant, events) {
    const sessions = this.build(tenant, events);
    return sessions.map((session) => ({
      id: session.id,
      events: session.events.length,
      seconds: this.duration(session),
    }));
  }

  stats(tenant) {
    const sessions = this.list(tenant);
    return {
      sessions: sessions.length,
      events: sessions.reduce((total, session) => total + session.events.length, 0),
      average_events: sessions.length === 0 ? 0 : sessions.reduce((total, session) => total + session.events.length, 0) / sessions.length,
    };
  }
}

export function sessionBucket(session) {
  return bucketOf(session.started_at, 'hour');
}

export function bounceRate(sessions) {
  if (sessions.length === 0) return 0;
  const bounced = sessions.filter((session) => session.events.length <= 1).length;
  return Math.round((bounced / sessions.length) * 10000) / 100;
}
