import { badRequest, conflict, invalidField, notFound } from '../http/errors.js';
import { applyFilters, paginate, sortRows, schemaFor } from '../store/query.js';
import { defaultsFor, mergeSettings } from './settings.js';
import { recordAudit } from './audit.js';
import { newId, slugify } from '../util/ids.js';

export const PLANS = ['free', 'team', 'business', 'enterprise'];
export const MAX_TENANTS_PER_USER = 200;

export class TenantService {
  constructor(options = {}) {
    this.store = options.store;
    if (!this.store) throw new TypeError('a tenant store is required');
    this.settings = options.settings;
    this.clock = options.clock ?? (() => new Date());
  }

  planOf(value) {
    const plan = String(value ?? 'free');
    if (!PLANS.includes(plan)) {
      throw invalidField('plan', `must be one of ${PLANS.join(', ')}`);
    }
    return plan;
  }

  slugOf(name, wanted) {
    const slug = slugify(wanted ?? name);
    if (slug.length < 2) throw invalidField('name', 'is too short to build a slug');
    return slug;
  }

  uniqueSlug(slug, ignoreId) {
    const clash = this.store.first('tenants', (row) => row.slug === slug && row.id !== ignoreId);
    if (clash) throw conflict(`slug ${slug} is already used by ${clash.id}`);
    return slug;
  }

  create(input, actor = 'system') {
    if (input === null || typeof input !== 'object') {
      throw badRequest('a tenant body is required');
    }
    const name = String(input.name ?? '').trim();
    if (name.length < 2 || name.length > 120) {
      throw invalidField('name', 'must be between 2 and 120 characters');
    }
    const slug = this.uniqueSlug(this.slugOf(name, input.slug));
    const settings = mergeSettings('tenant', defaultsFor('tenant'), input.settings ?? {});
    const row = this.store.insert('tenants', {
      id: newId('tnt'),
      slug,
      name,
      plan: this.planOf(input.plan),
      settings,
    });
    recordAudit(this.store, {
      tenantId: row.id,
      actor,
      action: 'tenant.create',
      target: row.id,
      at: this.clock(),
    });
    return row;
  }

  get(id) {
    const tenant = this.store.get('tenants', id);
    if (!tenant) throw notFound(`tenant ${id}`);
    return tenant;
  }

  bySlug(slug) {
    const tenant = this.store.first('tenants', (row) => row.slug === slug);
    if (!tenant) throw notFound(`tenant ${slug}`);
    return tenant;
  }

  list(query = {}) {
    const schema = schemaFor('tenants');
    let rows = this.store.all('tenants');
    if (query.q) {
      const needle = String(query.q).toLowerCase();
      rows = rows.filter(
        (row) => row.name.toLowerCase().includes(needle) || row.slug.includes(needle),
      );
    }
    rows = applyFilters(rows, { plan: query.plan }, schema);
    rows = sortRows(rows, { sort: query.sort ?? 'created_at', dir: query.dir }, schema);
    const page = paginate(rows, { limit: query.limit, offset: query.offset });
    return {
      rows: page.rows.map((row) => this.present(row)),
      total: page.total,
      next: page.next,
    };
  }

  update(id, patch, actor = 'system') {
    const tenant = this.get(id);
    if (patch === null || typeof patch !== 'object') throw badRequest('a tenant patch is required');
    const next = {};
    if (patch.name !== undefined) {
      const name = String(patch.name).trim();
      if (name.length < 2 || name.length > 120) {
        throw invalidField('name', 'must be between 2 and 120 characters');
      }
      next.name = name;
    }
    if (patch.slug !== undefined) {
      next.slug = this.uniqueSlug(this.slugOf(tenant.name, patch.slug), id);
    }
    if (patch.plan !== undefined) {
      next.plan = this.planOf(patch.plan);
    }
    const updated = this.store.update('tenants', id, (row) => {
      Object.assign(row, next);
      return row;
    });
    recordAudit(this.store, {
      tenantId: id,
      actor,
      action: 'tenant.update',
      target: id,
      at: this.clock(),
    });
    return updated;
  }

  updateSettings(id, patch) {
    this.get(id);
    return this.settings.updateTenant(id, patch);
  }

  settingsOf(id) {
    this.get(id);
    return this.settings.getTenant(id);
  }

  userCount(id) {
    return this.store.count('users') === 0
      ? 0
      : this.store.find('users', (row) => row.tenant_id === id).length;
  }

  present(tenant) {
    return {
      id: tenant.id,
      slug: tenant.slug,
      name: tenant.name,
      plan: tenant.plan,
      users: this.userCount(tenant.id),
      created_at: tenant.created_at,
      updated_at: tenant.updated_at,
    };
  }

  remove(id, actor = 'system') {
    const tenant = this.get(id);
    const users = this.store.find('users', (row) => row.tenant_id === id);
    for (const user of users) this.store.remove('users', user.id);
    this.store.remove('tenants', tenant.id);
    recordAudit(this.store, {
      tenantId: id,
      actor,
      action: 'tenant.remove',
      target: id,
      at: this.clock(),
    });
    return { id: tenant.id, removed: true, users: users.length };
  }

  ensureCapacity(limit = MAX_TENANTS_PER_USER) {
    const total = this.store.count('tenants');
    if (total >= limit) {
      throw conflict(`the workspace holds at most ${limit} tenants`);
    }
    return total;
  }
}
