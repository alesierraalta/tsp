import { newId } from '../util/ids.js';
import { isPast, nextRun, toIso } from '../util/time.js';
import { notFound } from '../util/errors.js';

export class Scheduler {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.tasks = new Map();
    this.runs = [];
  }

  add(input = {}) {
    const task = {
      id: newId('task'),
      name: String(input.name ?? 'task'),
      kind: String(input.kind ?? 'rerender'),
      every_minutes: Math.max(1, Math.trunc(input.every_minutes ?? 60)),
      enabled: input.enabled !== false,
      last_run_at: input.last_run_at ? toIso(input.last_run_at) : null,
      next_run_at: toIso(input.next_run_at) ?? nextRun(input.last_run_at ?? this.clock(), input.every_minutes ?? 60, this.clock).toISOString(),
    };
    this.tasks.set(task.id, task);
    return task;
  }

  get(id) {
    const found = this.tasks.get(String(id));
    if (!found) throw notFound(`task ${id}`);
    return found;
  }

  list() {
    return [...this.tasks.values()].sort((left, right) => (left.next_run_at < right.next_run_at ? -1 : 1));
  }

  due(at) {
    return this.list().filter((task) => task.enabled && isPast(task.next_run_at, () => at ?? this.clock()));
  }

  markRun(id, at) {
    const task = this.get(id);
    const instant = at ?? this.clock();
    task.last_run_at = toIso(instant);
    task.next_run_at = nextRun(instant, task.every_minutes, this.clock).toISOString();
    this.runs.push({ task: task.id, at: task.last_run_at });
    return task;
  }

  enable(id) {
    const task = this.get(id);
    task.enabled = true;
    return task;
  }

  disable(id) {
    const task = this.get(id);
    task.enabled = false;
    return task;
  }

  remove(id) {
    return this.tasks.delete(String(id));
  }

  async tick(run, at) {
    const due = this.due(at);
    const results = [];
    for (const task of due) {
      try {
        results.push({ task: task.id, ok: true, result: await run(task) });
      } catch (error) {
        results.push({ task: task.id, ok: false, error: error.message });
      }
      this.markRun(task.id, at);
    }
    return results;
  }

  stats() {
    return {
      tasks: this.tasks.size,
      enabled: this.list().filter((task) => task.enabled).length,
      runs: this.runs.length,
    };
  }

  upcoming(limit = 5) {
    return this.list().filter((task) => task.enabled).slice(0, limit);
  }

  overdue(at) {
    const when = at ?? this.clock();
    return this.list().filter((task) => task.enabled && isPast(task.next_run_at, () => when)).length;
  }

  byKind(kind) {
    return this.list().filter((task) => task.kind === kind);
  }

  history(limit = 20) {
    return this.runs.slice(-limit).reverse();
  }
}
