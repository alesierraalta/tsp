export class AppError extends Error {
  constructor(code, message, { status = 400, details } = {}) {
    super(message);
    this.name = 'AppError';
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

export function isAppError(error) {
  return error instanceof AppError;
}

export function badRequest(code, message, details) {
  return new AppError(code, message, { status: 400, details });
}

export function unauthorized(message = 'authentication required') {
  return new AppError('unauthorized', message, { status: 401 });
}

export function forbidden(message = 'not allowed') {
  return new AppError('forbidden', message, { status: 403 });
}

export function notFound(label) {
  return new AppError('not_found', `${label} not found`, { status: 404 });
}

export function conflict(code, message) {
  return new AppError(code, message, { status: 409 });
}

export function tooLarge(message) {
  return new AppError('payload_too_large', message, { status: 413 });
}

export function unsafePath(name) {
  return new AppError('unsafe_path', `unsafe path ${JSON.stringify(String(name))}`, { status: 400 });
}

export function invalidField(field, reason) {
  return new AppError('invalid_field', `${field} ${reason}`, { status: 400, details: { field } });
}

export function unsupported(label) {
  return new AppError('unsupported', `${label} is not supported`, { status: 422 });
}

export function asError(error) {
  if (error instanceof Error) return error;
  return new Error(String(error));
}

export function statusOf(error) {
  return isAppError(error) ? error.status : 500;
}

export function codeOf(error) {
  return isAppError(error) ? error.code : 'internal_error';
}

export function wrapError(error, code, message) {
  const original = asError(error);
  const wrapped = new AppError(code, message ?? original.message, { status: 500 });
  wrapped.cause = original;
  return wrapped;
}
