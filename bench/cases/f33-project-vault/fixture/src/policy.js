import { identifier, role } from './values.js';

const ranks = { reader: 1, editor: 2, owner: 3 };

export class Policy {
  constructor(store) {
    this.store = store;
  }

  snapshot(projectId) {
    const project = this.store.project(projectId);
    return Object.freeze({
      project: project.id,
      revision: project.revision,
      members: Object.freeze({ ...project.members }),
    });
  }

  allows(snapshot, actor, required = 'reader') {
    identifier(actor);
    role(required);
    return (ranks[snapshot.members[actor]] ?? 0) >= ranks[required];
  }

  require(projectId, actor, required = 'reader') {
    const snapshot = this.snapshot(projectId);
    if (!this.allows(snapshot, actor, required)) throw new Error('forbidden');
    return snapshot;
  }

  members(projectId, actor) {
    const snapshot = this.require(projectId, actor);
    return Object.entries(snapshot.members)
      .map(([user, access]) => ({ user, access }))
      .sort((a, b) => a.user.localeCompare(b.user));
  }

  grant(projectId, actor, user, access) {
    this.require(projectId, actor, 'owner');
    identifier(user);
    role(access);
    return this.store.setMember(projectId, user, access);
  }

  revoke(projectId, actor, user) {
    const snapshot = this.require(projectId, actor, 'owner');
    identifier(user);
    if (snapshot.members[user] === 'owner') throw new Error('owner is permanent');
    return this.store.setMember(projectId, user, null);
  }
}
