export class DashboardError extends Error {
  constructor(code, message, { status = 400, details } = {}) {
    super(message);
    this.name = 'DashboardError';
    this.code = code;
    this.status = status;
    this.details = details;
  }

  toJSON() {
    const body = { error: this.code, message: this.message };
    if (this.details !== undefined) body.details = this.details;
    return body;
  }
}

export function badRequest(code, message, details) {
  return new DashboardError(code, message, { status: 400, details });
}

export function unauthorized(message = 'an api key is required') {
  return new DashboardError('unauthorized', message, { status: 401 });
}

export function forbidden(message = 'not allowed') {
  return new DashboardError('forbidden', message, { status: 403 });
}

export function notFound(label) {
  return new DashboardError('not_found', `${label} not found`, { status: 404 });
}

export function conflict(code, message, details) {
  return new DashboardError(code, message, { status: 409, details });
}

export function tooLarge(message, details) {
  return new DashboardError('payload_too_large', message, { status: 413, details });
}

export function unsupported(message) {
  return new DashboardError('unsupported', message, { status: 422 });
}

export function isDashboardError(error) {
  return error instanceof DashboardError;
}

export function statusOf(error) {
  if (isDashboardError(error)) return error.status;
  if (typeof error?.status === 'number') return error.status;
  return 500;
}

export function codeOf(error) {
  if (isDashboardError(error)) return error.code;
  if (typeof error?.code === 'string') return error.code;
  return 'internal_error';
}

export function wrap(error, code, message) {
  const wrapped = new DashboardError(code, message ?? String(error?.message ?? error), { status: 500 });
  wrapped.cause = error;
  return wrapped;
}
