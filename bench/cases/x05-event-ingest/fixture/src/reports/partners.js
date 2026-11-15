import { createHash } from 'node:crypto';
import { toCsvCell } from './csv.js';
import { stableStringify } from '../util/json.js';
import { readPath } from '../store/query.js';
import { badRequest } from '../util/errors.js';
import { slugify, singleLine } from '../util/text.js';
import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

// Partners read a tenant's events through a fixed field list. The list is what makes the export safe:
// a partner never receives a field it did not ask for, and the paths in PRIVATE_PATHS are refused at
// definition time because they identify a person rather than an event.
export const PARTNER_KINDS = ['agency', 'reseller', 'analytics'];
export const DEFAULT_FIELDS = ['name', 'at', 'properties.path'];
export const PRIVATE_PATHS = ['context.user', 'context.referrer', 'context.session'];
export const PARTNER_FORMATS = ['csv', 'ndjson', 'json'];

export function columnOf(path) {
  return String(path).replace(/\./g, '_');
}

export function blockedFields() {
  return [...PRIVATE_PATHS];
}

export function definePartner(input = {}, options = {}) {
  const name = singleLine(input.name ?? '');
  if (name === '') throw badRequest('invalid_partner', 'a partner needs a name');
  const kind = PARTNER_KINDS.includes(input.kind) ? input.kind : 'analytics';
  const asked = Array.isArray(input.fields) && input.fields.length > 0 ? input.fields.map(String) : [...DEFAULT_FIELDS];
  const refused = asked.filter((field) => PRIVATE_PATHS.includes(field));
  if (refused.length > 0) {
    throw badRequest('partner_field_blocked', `${refused[0]} is never exported to a partner`, { fields: refused });
  }
  const fields = [...new Set(asked)];
  if (fields.length > 12) throw badRequest('partner_field_limit', 'a partner asks for at most 12 fields');
  return {
    id: options.id ?? (slugify(name) || newId('prt')),
    name,
    kind,
    fields,
    columns: fields.map(columnOf),
    tenants: Array.isArray(input.tenants) ? input.tenants.map(String) : [],
    created_at: nowIso(options.clock),
  };
}

export function partnerRow(partner, event) {
  const row = {};
  for (const field of partner.fields) {
    const value = readPath(event, field);
    row[columnOf(field)] = value === undefined ? null : value;
  }
  return row;
}

export function partnerRows(partner, events) {
  return (events ?? []).map((event) => partnerRow(partner, event));
}

export function checksumOf(text) {
  return createHash('sha256').update(String(text ?? ''), 'utf8').digest('hex');
}

export function renderPartnerRows(partner, rows, format) {
  if (!PARTNER_FORMATS.includes(format)) throw new TypeError(`unknown partner format ${format}`);
  if (format === 'csv') {
    const header = partner.columns.map(toCsvCell).join(',');
    const body = rows.map((row) => partner.columns.map((column) => toCsvCell(row[column])).join(','));
    return `${[header, ...body].join('\n')}\n`;
  }
  if (format === 'ndjson') return `${rows.map((row) => JSON.stringify(row)).join('\n')}\n`;
  return stableStringify({ columns: partner.columns, rows });
}

export function partnerManifest(partner, rows, format, options = {}) {
  const text = renderPartnerRows(partner, rows, format);
  return {
    partner: partner.id,
    kind: partner.kind,
    format,
    columns: [...partner.columns],
    rows: rows.length,
    bytes: Buffer.byteLength(text, 'utf8'),
    checksum: checksumOf(text),
    generated_at: nowIso(options.clock),
  };
}

export function stampOf(clock = () => new Date()) {
  return new Date(clock()).toISOString().slice(0, 10).replace(/-/g, '');
}

export class PartnerBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.partners = new Map();
    this.files = new Map();
    this.exports = [];
  }

  define(input) {
    const partner = definePartner(input, { clock: this.clock });
    if (this.partners.has(partner.id)) throw badRequest('partner_exists', `partner ${partner.id} already exists`);
    this.partners.set(partner.id, partner);
    return partner;
  }

  get(id) {
    return this.partners.get(String(id));
  }

  require(id) {
    const partner = this.get(id);
    if (!partner) throw badRequest('partner_missing', `partner ${id} does not exist`);
    return partner;
  }

  list() {
    return [...this.partners.values()];
  }

  forTenant(tenant) {
    return this.list().filter((partner) => partner.tenants.includes(tenant));
  }

  assign(partnerId, tenant) {
    const partner = this.require(partnerId);
    if (!partner.tenants.includes(tenant)) partner.tenants.push(tenant);
    return partner;
  }

  remove(id) {
    return this.partners.delete(String(id));
  }

  export(partnerId, events, options = {}) {
    const partner = this.require(partnerId);
    const format = PARTNER_FORMATS.includes(options.format) ? options.format : 'csv';
    const rows = partnerRows(partner, events);
    const manifest = partnerManifest(partner, rows, format, { clock: this.clock });
    const text = renderPartnerRows(partner, rows, format);
    const name = `${partner.id}-${stampOf(this.clock)}-${String(this.exports.length + 1).padStart(3, '0')}.${format}`;
    if (options.store !== false) {
      this.files.set(name, { name, partner: partner.id, format, text, manifest });
      this.exports.push({ partner: partner.id, name, at: manifest.generated_at, checksum: manifest.checksum });
    }
    return { name, manifest, text };
  }

  file(partnerId, name) {
    const found = this.files.get(String(name));
    if (!found) return undefined;
    return found.partner === String(partnerId) ? found : undefined;
  }

  filesOf(partnerId) {
    return [...this.files.values()]
      .filter((file) => file.partner === String(partnerId))
      .map((file) => ({ name: file.name, format: file.format, rows: file.manifest.rows, checksum: file.manifest.checksum }));
  }

  history(partnerId) {
    return this.exports.filter((entry) => partnerId === undefined || entry.partner === String(partnerId));
  }

  stats() {
    return { partners: this.partners.size, files: this.files.size, exports: this.exports.length };
  }
}

export function bundleFiles(partner, rows, options = {}) {
  const format = PARTNER_FORMATS.includes(options.format) ? options.format : 'csv';
  const size = Math.max(1, Math.trunc(options.size ?? 500));
  const groups = [];
  for (let index = 0; index < rows.length; index += size) groups.push(rows.slice(index, index + size));
  const files = groups.map((slice, index) => {
    const text = renderPartnerRows(partner, slice, format);
    return {
      name: `${partner.id}-${String(index + 1).padStart(3, '0')}.${format}`,
      rows: slice.length,
      bytes: Buffer.byteLength(text, 'utf8'),
      checksum: checksumOf(text),
      text,
    };
  });
  return {
    files,
    rows: rows.length,
    bytes: files.reduce((total, file) => total + file.bytes, 0),
    checksum: checksumOf(files.map((file) => file.checksum).join('')),
  };
}

export function partnerSummary(partner, rows) {
  return {
    partner: partner.id,
    kind: partner.kind,
    columns: partner.columns.length,
    rows: (rows ?? []).length,
    tenants: [...partner.tenants],
  };
}
