export class ValidateError extends Error {
  constructor(message) {
    super(message);
    this.name = 'ValidateError';
    this.code = 'invalid_field';
    this.status = 400;
  }

  toJSON() {
    return { error: this.code, message: this.message };
  }
}

export function assertShape(value, shape, label = 'value') {
  if (shape === 'object') {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) {
      throw new ValidateError(`${label} must be an object`);
    }
    return true;
  }
  if (shape === 'list') {
    if (!Array.isArray(value)) throw new ValidateError(`${label} must be a list`);
    return true;
  }
  return true;
}
