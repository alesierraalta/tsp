export class Sessions {
  #tokens = new Map();
  #queue = new Map();
  #next = 0;
  #effects = [];
  constructor(now = () => Date.now()) {
    this.now = now;
  }
  issue(subject, ttl) {
    if (!(ttl > 0)) throw new Error('invalid ttl');
    const token = `session-${++this.#next}`;
    this.#tokens.set(token, { subject, expires: this.now() + ttl, revoked: false });
    return token;
  }
  revoke(token) {
    const session = this.#tokens.get(token);
    if (session) session.revoked = true;
  }
  refresh(token, ttl) {
    const session = this.#tokens.get(token);
    if (!session || session.revoked || session.expires <= this.now()) throw new Error('denied');
    return this.issue(session.subject, ttl);
  }
  submit(token, message) {
    const session = this.#valid(token);
    const ticket = Symbol();
    this.#queue.set(ticket, { token, subject: session.subject, message });
    return ticket;
  }
  dispatch(ticket) {
    const job = this.#queue.get(ticket);
    if (!job) throw new Error('unknown ticket');
    const session = this.#tokens.get(job.token);
    if (!session || session.revoked || session.expires <= this.now()) throw new Error('denied');
    this.#effects.push({ subject: job.subject, message: job.message });
    this.#queue.delete(ticket);
  }
  effects() {
    return this.#effects.map(effect => ({ ...effect }));
  }
  #valid(token) {
    const session = this.#tokens.get(token);
    if (!session || session.revoked || session.expires <= this.now()) throw new Error('denied');
    return session;
  }
}
