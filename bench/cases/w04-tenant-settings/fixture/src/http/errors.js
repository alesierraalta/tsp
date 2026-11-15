export class HttpError extends Error {
  constructor(status, code, message, details) {
    super(message);
    this.name = 'HttpError';
    this.status = status;
    this.code = code;
    if (details !== undefined) this.details = details;
  }

  toJSON() {
    const body = { error: this.code, message: this.message };
    if (this.details !== undefined) body.details = this.details;
    return body;
  }
}

export function badRequest(message, details) {
  return new HttpError(400, 'bad_request', message, details);
}

export function invalidField(field, reason) {
  return new HttpError(400, 'invalid_field', `${field}: ${reason}`, { field, reason });
}

export function unauthorized(message = 'authentication required') {
  return new HttpError(401, 'unauthorized', message);
}

export function forbidden(message = 'not allowed') {
  return new HttpError(403, 'forbidden', message);
}

export function notFound(what = 'resource') {
  return new HttpError(404, 'not_found', `${what} was not found`);
}

export function conflict(message) {
  return new HttpError(409, 'conflict', message);
}

export function tooLarge(limit) {
  return new HttpError(413, 'body_too_large', `request body exceeds ${limit} bytes`);
}

export function unsupportedMedia(type) {
  return new HttpError(415, 'unsupported_media', `unsupported content type: ${type}`);
}

export function unsafePath(name) {
  return new HttpError(400, 'unsafe_path', `path ${JSON.stringify(String(name))} is not a plain file name`);
}

export function methodNotAllowed(method, path) {
  return new HttpError(405, 'method_not_allowed', `${method} is not allowed on ${path}`);
}

export function isHttpError(error) {
  return error instanceof HttpError;
}

export function asHttpError(error) {
  if (error instanceof HttpError) return error;
  return new HttpError(500, 'internal_error', 'internal error');
}
