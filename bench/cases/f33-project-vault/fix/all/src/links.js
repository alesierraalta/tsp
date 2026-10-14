import { identifier, timestamp } from './values.js';

export class Links {
  #links = new Map();
  #next = 0;

  constructor(workspace, clock = () => Date.now()) {
    this.workspace = workspace;
    this.clock = clock;
  }

  issue(actor, document, recipient, expiresAt) {
    const value = this.workspace.read(actor, document);
    this.workspace.policy.require(value.project, actor, 'editor');
    identifier(recipient);
    timestamp(expiresAt);
    if (expiresAt <= this.clock()) throw new Error('expired');
    const token = `link-${++this.#next}`;
    this.#links.set(token, { actor, document, recipient, expiresAt, revoked: false });
    this.workspace.journal.append('share', actor, document, { recipient });
    return token;
  }

  revoke(actor, token) {
    const link = this.#links.get(token);
    if (!link) throw new Error('unknown link');
    const document = this.workspace.store.document(link.document);
    this.workspace.policy.require(document.project, actor, 'editor');
    link.revoked = true;
    this.workspace.journal.append('revoke', actor, link.document);
  }

  renew(actor, token, expiresAt) {
    const link = this.#links.get(token);
    if (!link) throw new Error('unknown link');
    const document = this.workspace.store.document(link.document);
    this.workspace.policy.require(document.project, actor, 'editor');
    timestamp(expiresAt);
    if (expiresAt <= this.clock()) throw new Error('expired');
    link.expiresAt = expiresAt;
    return token;
  }

  read(recipient, token) {
    const link = this.#links.get(token);
    if (!link || link.recipient !== recipient) throw new Error('forbidden');
    if (link.revoked || this.clock() >= link.expiresAt) throw new Error('expired');
    return this.workspace.store.document(link.document);
  }

  inspect(actor, token) {
    const link = this.#links.get(token);
    if (!link || link.actor !== actor) throw new Error('forbidden');
    return { document: link.document, recipient: link.recipient,
      expiresAt: link.expiresAt, revoked: link.revoked };
  }
}
