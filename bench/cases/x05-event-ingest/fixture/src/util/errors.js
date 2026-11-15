export class IngestError extends Error {
  constructor(code, message, { status = 400, details } = {}) {
    super(message);
    this.name = 'IngestError';
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
  return new IngestError(code, message, { status: 400, details });
}

export function unauthorized(message = 'an api key is required') {
  return new IngestError('unauthorized', message, { status: 401 });
}

export function forbidden(message = 'not allowed') {
  return new IngestError('forbidden', message, { status: 403 });
}

export function notFound(label) {
  return new IngestError('not_found', `${label} not found`, { status: 404 });
}

export function conflict(code, message) {
  return new IngestError(code, message, { status: 409 });
}

export function tooLarge(message) {
  return new IngestError('payload_too_large', message, { status: 413 });
}

export function unsupported(label) {
  return new IngestError('unsupported', `${label} is not supported`, { status: 422 });
}

export function isIngestError(error) {
  return error instanceof IngestError;
}

export function statusOf(error) {
  return isIngestError(error) ? error.status : 500;
}

export function codeOf(error) {
  return isIngestError(error) ? error.code : 'internal_error';
}

export function wrap(error, code, message) {
  const wrapped = new IngestError(code, message ?? String(error?.message ?? error), { status: 500 });
  wrapped.cause = error;
  return wrapped;
}
