export class RelayError extends Error {
  constructor(code, message, details) {
    super(message);
    this.name = 'RelayError';
    this.code = code;
    this.retryable = false;
    if (details !== undefined) this.details = details;
  }

  toJSON() {
    const body = { error: this.code, message: this.message };
    if (this.details !== undefined) body.details = this.details;
    return body;
  }
}

export class EgressError extends RelayError {
  constructor(message, details) {
    super('egress_blocked', `egress blocked: ${message}`, details);
    this.name = 'EgressError';
  }
}

export class DeliveryError extends RelayError {
  constructor(message, details) {
    super('delivery_failed', message, details);
    this.name = 'DeliveryError';
    this.retryable = true;
  }
}

export function notFound(what) {
  return new RelayError('not_found', `${what} was not found`);
}

export function conflict(message) {
  return new RelayError('conflict', message);
}

export function hookFailed(message, details) {
  const error = new RelayError('hook_failed', message, details);
  error.retryable = false;
  return error;
}

export function isRelayError(error) {
  return error instanceof RelayError;
}

export function asRelayError(error) {
  if (error instanceof RelayError) return error;
  return new RelayError('internal_error', 'the relay failed');
}
