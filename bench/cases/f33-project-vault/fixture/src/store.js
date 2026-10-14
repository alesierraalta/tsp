import { identifier, title, labels, copy, page } from './values.js';

export class Store {
  #projects = new Map();
  #documents = new Map();

  createProject(id, owner) {
    id = identifier(id);
    owner = identifier(owner);
    if (this.#projects.has(id)) throw new Error('project exists');
    const project = { id, members: { [owner]: 'owner' }, revision: 0 };
    this.#projects.set(id, project);
    return copy(project);
  }

  project(id) {
    const value = this.#projects.get(id);
    if (!value) throw new Error('unknown project');
    return copy(value);
  }

  setMember(projectId, user, role) {
    const project = this.#projects.get(projectId);
    if (!project) throw new Error('unknown project');
    if (role === null) delete project.members[user];
    else project.members[user] = role;
    project.revision += 1;
    return copy(project);
  }

  createDocument(id, project, name, tags = []) {
    id = identifier(id);
    this.project(project);
    if (this.#documents.has(id)) throw new Error('document exists');
    const value = { id, project, title: title(name), labels: labels(tags), revision: 0 };
    this.#documents.set(id, value);
    return copy(value);
  }

  document(id) {
    const value = this.#documents.get(id);
    if (!value) throw new Error('unknown document');
    return copy(value);
  }

  update(id, changes) {
    const value = this.#documents.get(id);
    if (!value) throw new Error('unknown document');
    if ('title' in changes) value.title = title(changes.title);
    if ('labels' in changes) value.labels = labels(changes.labels);
    value.revision += 1;
    return copy(value);
  }

  list(project, options) {
    this.project(project);
    const { offset, limit } = page(options);
    return [...this.#documents.values()].filter(d => d.project === project)
      .sort((a, b) => a.id.localeCompare(b.id)).slice(offset, offset + limit).map(copy);
  }
}
