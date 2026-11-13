import { conflict, notFound } from '../util/errors.js';
import { canonical } from '../util/text.js';

const TOPIC_NAME = /^[a-z][a-z0-9._-]{0,48}$/;

export const DEFAULT_TOPICS = [
  { name: 'default', event: 'generic', target: 'primary' },
  { name: 'orders', event: 'order.created', target: 'primary' },
  { name: 'items', event: 'item.updated', target: 'primary' },
  { name: 'imports', event: 'import.finished', target: 'hooks' },
];

export function validateTopicName(name) {
  const text = canonical(name);
  if (!TOPIC_NAME.test(text)) throw new TypeError(`not a topic name: ${name}`);
  return text;
}

export class TopicRegistry {
  constructor(topics = []) {
    this.topics = new Map();
    for (const topic of topics) this.add(topic);
  }

  add(topic) {
    const name = validateTopicName(topic?.name);
    if (this.topics.has(name)) throw conflict(`topic ${name} already exists`);
    const stored = {
      name,
      event: String(topic.event ?? 'generic'),
      target: String(topic.target ?? 'primary'),
      maxAttempts: topic.maxAttempts,
      enabled: topic.enabled !== false,
    };
    this.topics.set(name, stored);
    return stored;
  }

  get(name) {
    const topic = this.topics.get(canonical(name));
    if (!topic) throw notFound(`topic ${name}`);
    return topic;
  }

  find(name) {
    return this.topics.get(canonical(name));
  }

  forEvent(event) {
    const wanted = canonical(event);
    return [...this.topics.values()].filter((topic) => canonical(topic.event) === wanted && topic.enabled);
  }

  disable(name) {
    const topic = this.get(name);
    topic.enabled = false;
    return topic;
  }

  enable(name) {
    const topic = this.get(name);
    topic.enabled = true;
    return topic;
  }

  remove(name) {
    const topic = this.get(name);
    this.topics.delete(topic.name);
    return topic;
  }

  list() {
    return [...this.topics.values()].sort((left, right) => (left.name < right.name ? -1 : 1));
  }

  names() {
    return this.list().map((topic) => topic.name);
  }

  describe() {
    return this.list().map((topic) => `${topic.name}:${topic.event}:${topic.enabled ? 'on' : 'off'}`);
  }

  count() {
    return this.topics.size;
  }
}

export function fanoutPlan(topics, event, entity) {
  return topics
    .forEvent(event)
    .map((topic) => ({
      topic: topic.name,
      target: topic.target,
      event: topic.event,
      entity_id: entity,
      maxAttempts: topic.maxAttempts,
    }));
}

export function topicSummary(registry) {
  return registry.list().map((topic) => `${topic.name}=${registry.list().length ? topic.event : ''}`);
}

export function mergeTopic(registry, name, patch) {
  const topic = registry.get(name);
  const merged = { ...topic, ...patch, name: topic.name };
  registry.remove(topic.name);
  return registry.add(merged);
}

export function planFrom(registry, event, entity, options = {}) {
  const plan = fanoutPlan(registry, event, entity);
  const limited = options.limit ? plan.slice(0, options.limit) : plan;
  return limited.map((entry) => ({ ...entry, dry_run: Boolean(options.dryRun) }));
}

export function findTopicForTarget(registry, target) {
  return registry.list().find((topic) => topic.target === target);
}
