export class ImportError extends Error {
  constructor(code, message, details) {
    super(message);
    this.name = 'ImportError';
    this.code = code;
    if (details !== undefined) this.details = details;
  }

  toJSON() {
    const body = { error: this.code, message: this.message };
    if (this.details !== undefined) body.details = this.details;
    return body;
  }
}

export function schemaError(message, details) {
  return new ImportError('schema_error', message, details);
}

export function invalidRow(rowNumber, problems) {
  return new ImportError('invalid_row', `row ${rowNumber} is not valid`, { row: rowNumber, problems });
}

export function exportError(message, details) {
  return new ImportError('export_error', message, details);
}

export function notFound(what) {
  return new ImportError('not_found', `${what} was not found`);
}

export function tooManyRows(limit) {
  return new ImportError('too_many_rows', `the file holds more than ${limit} rows`);
}

export function isImportError(error) {
  return error instanceof ImportError;
}

export function problem(field, message) {
  return { field, message };
}

export function summarizeProblems(problems) {
  if (!Array.isArray(problems) || problems.length === 0) return 'no problems';
  return problems.map((entry) => `${entry.field}: ${entry.message}`).join('; ');
}

export function asImportError(error) {
  if (error instanceof ImportError) return error;
  return new ImportError('internal_error', 'the import failed');
}
