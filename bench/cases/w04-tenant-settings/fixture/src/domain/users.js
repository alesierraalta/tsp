import { badRequest, conflict, forbidden, invalidField, notFound } from '../http/errors.js';
import { applyFilters, paginate, schemaFor, sortRows } from '../store/query.js';
import { defaultsFor, mergeSettings } from './settings.js';
import { recordAudit } from './audit.js';
import { newId } from '../util/ids.js';

export const ROLES = ['owner', 'admin', 'member', 'viewer'];
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[a-z]{2,}$/i;

export function canonicalEmail(value) {
  const text = String(value ?? '').normalize('NFKC').trim();
  return text.toLowerCase();
}

export class UserService {
  constructor(options = {}) {
    this.store = options.store;
    if (!this.store) throw new TypeError('a user store is required');
    this.settings = options.settings;
    this.clock = options.clock ?? (() => new Date());
    this.tenants = options.tenants;
  }

  emailOf(value) {
    const email = canonicalEmail(value);
    if (email.length > 254 || !EMAIL_PATTERN.test(email)) {
      throw invalidField('email', 'is not an email address');
    }
    return email;
  }

  rolesOf(value) {
    const roles = Array.isArray(value) ? value : [value].filter(Boolean);
    const cleaned = [...new Set(roles.map((role) => String(role).toLowerCase()))];
    if (cleaned.length === 0) throw invalidField('roles', 'at least one role is required');
    for (const role of cleaned) {
      if (!ROLES.includes(role)) throw invalidField('roles', `unknown role ${role}`);
    }
    return cleaned;
  }

  assertTenant(tenantId) {
    if (this.tenants) this.tenants.get(tenantId);
    else if (!this.store.get('tenants', tenantId)) throw notFound(`tenant ${tenantId}`);
    return tenantId;
  }

  create(input, actor = 'system') {
    if (input === null || typeof input !== 'object') throw badRequest('a user body is required');
    const tenantId = String(input.tenant_id ?? input.tenantId ?? '');
    this.assertTenant(tenantId);
    const email = this.emailOf(input.email);
    const existing = this.store.first('users', (row) => row.email === email);
    if (existing) throw conflict(`email ${email} is already registered`);
    const name = String(input.name ?? '').trim();
    if (name.length < 1 || name.length > 120) {
      throw invalidField('name', 'must be between 1 and 120 characters');
    }
    const row = this.store.insert('users', {
      id: newId('usr'),
      tenant_id: tenantId,
      email,
      name,
      roles: this.rolesOf(input.roles ?? ['member']),
      active: true,
      settings: mergeSettings('user', defaultsFor('user'), input.settings ?? {}),
    });
    recordAudit(this.store, {
      tenantId,
      actor,
      action: 'user.create',
      target: row.id,
      at: this.clock(),
    });
    return row;
  }

  get(id) {
    const user = this.store.get('users', id);
    if (!user) throw notFound(`user ${id}`);
    return user;
  }

  byEmail(email) {
    const canonical = canonicalEmail(email);
    const user = this.store.first('users', (row) => row.email === canonical);
    if (!user) throw notFound(`user ${canonical}`);
    return user;
  }

  list(tenantId, query = {}) {
    const schema = schemaFor('users');
    let rows = this.store.find('users', (row) => row.tenant_id === tenantId);
    if (query.q) {
      const needle = String(query.q).toLowerCase();
      rows = rows.filter(
        (row) => row.email.includes(needle) || row.name.toLowerCase().includes(needle),
      );
    }
    rows = applyFilters(rows, { roles: undefined }, schema);
    if (query.role) {
      rows = rows.filter((row) => (row.roles ?? []).includes(query.role));
    }
    rows = sortRows(rows, { sort: query.sort ?? 'created_at', dir: query.dir }, schema);
    const page = paginate(rows, { limit: query.limit, offset: query.offset });
    return { rows: page.rows.map((row) => this.present(row)), total: page.total, next: page.next };
  }

  setRoles(id, roles, actor, principal) {
    const user = this.get(id);
    const next = this.rolesOf(roles);
    if (user.roles.includes('owner') && !next.includes('owner')) {
      const owners = this.store.find('users', (row) => row.tenant_id === user.tenant_id)
        .filter((row) => row.roles.includes('owner'));
      if (owners.length <= 1) {
        throw conflict('the last owner of a tenant cannot lose the owner role');
      }
    }
    if (principal && !principal.roles?.includes('platform') && user.id === principal.id) {
      throw forbidden('you cannot change your own roles');
    }
    const updated = this.store.update('users', id, (row) => {
      row.roles = next;
      return row;
    });
    recordAudit(this.store, {
      tenantId: user.tenant_id,
      actor: actor ?? 'system',
      action: 'user.roles',
      target: id,
      at: this.clock(),
    });
    return updated;
  }

  rename(id, name, actor = 'system') {
    const clean = String(name ?? '').trim();
    if (clean.length < 1 || clean.length > 120) {
      throw invalidField('name', 'must be between 1 and 120 characters');
    }
    const user = this.get(id);
    const updated = this.store.update('users', id, (row) => {
      row.name = clean;
      return row;
    });
    recordAudit(this.store, {
      tenantId: user.tenant_id,
      actor,
      action: 'user.rename',
      target: id,
      at: this.clock(),
    });
    return updated;
  }

  updateSettings(id, patch) {
    this.get(id);
    return this.settings.updateUser(id, patch);
  }

  settingsOf(id) {
    this.get(id);
    return this.settings.getUser(id);
  }

  deactivate(id, actor = 'system') {
    const user = this.get(id);
    const updated = this.store.update('users', id, (row) => {
      row.active = false;
      return row;
    });
    recordAudit(this.store, {
      tenantId: user.tenant_id,
      actor,
      action: 'user.deactivate',
      target: id,
      at: this.clock(),
    });
    return updated;
  }

  present(user) {
    return {
      id: user.id,
      tenant_id: user.tenant_id,
      email: user.email,
      name: user.name,
      roles: [...(user.roles ?? [])],
      active: user.active !== false,
      created_at: user.created_at,
    };
  }

  members(tenantId) {
    return this.store
      .find('users', (row) => row.tenant_id === tenantId && row.active !== false)
      .map((row) => this.present(row));
  }
}
