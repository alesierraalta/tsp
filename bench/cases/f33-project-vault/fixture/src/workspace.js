import { Store } from './store.js';
import { Policy } from './policy.js';
import { Journal } from './journal.js';
import { title, labels } from './values.js';

export class Workspace {
  #jobs = new Map();
  #next = 0;

  constructor(store = new Store(), journal = new Journal()) {
    this.store = store;
    this.policy = new Policy(store);
    this.journal = journal;
  }

  createProject(id, actor) {
    const value = this.store.createProject(id, actor);
    this.journal.append('project', actor, id);
    return value;
  }

  createDocument(actor, id, project, name, tags = []) {
    this.policy.require(project, actor, 'editor');
    const value = this.store.createDocument(id, project, name, tags);
    this.journal.append('create', actor, id);
    return value;
  }

  read(actor, id) {
    const document = this.store.document(id);
    this.policy.require(document.project, actor);
    return document;
  }

  prepare(actor, id, changes) {
    const document = this.store.document(id);
    const permission = this.policy.require(document.project, actor, 'editor');
    const patch = {};
    if ('title' in changes) patch.title = title(changes.title);
    if ('labels' in changes) patch.labels = labels(changes.labels);
    if (!Object.keys(patch).length) throw new Error('empty patch');
    const token = `job-${++this.#next}`;
    this.#jobs.set(token, { actor, id, permission, patch });
    return token;
  }

  apply(token) {
    const job = this.#jobs.get(token);
    if (!job) throw new Error('unknown job');
    if (!this.policy.allows(job.permission, job.actor, 'editor')) throw new Error('forbidden');
    const value = this.store.update(job.id, job.patch);
    this.#jobs.delete(token);
    this.journal.append('update', job.actor, job.id, { revision: value.revision });
    return value;
  }

  cancel(token) {
    return this.#jobs.delete(token);
  }
}
