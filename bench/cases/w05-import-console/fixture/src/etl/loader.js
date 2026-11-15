import { parseCsv } from './reader.js';
import { DATASETS, compareSchemas, keyColumns, schemaOf } from './schema.js';
import { countProblems, firstProblem, validateRows } from './validate.js';
import { normalizeMoney, normalizeRows, renameColumns } from './normalize.js';
import { Dataset } from './store.js';
import { LIMITS } from '../config.js';
import { ImportError, invalidRow } from '../util/errors.js';

export function loadDataset(name, csvText, options = {}) {
  const schema = schemaOf(name);
  const limit = options.limit ?? LIMITS.maxRows;
  const parsed = parseCsv(csvText, options);
  if (parsed.rows.length > limit) {
    throw new ImportError('too_many_rows', `the file holds more than ${limit} rows`);
  }
  const renamed = renameColumns(parsed.rows, parsed.header, schema);
  const validation = validateRows(schema, renamed, { limit, allowUnknown: options.allowUnknown });
  if (!validation.ok) {
    const problem = firstProblem(validation);
    throw invalidRow(problem ? validation.errors[0].row : 2, validation.errors[0]?.problems ?? [problem]);
  }
  const normalized = normalizeMoney(schema, normalizeRows(schema, renamed));
  const dataset = new Dataset(schema);
  dataset.replaceAll(normalized);
  return { dataset, validation, header: parsed.header, delimiter: parsed.delimiter };
}

export function importDataset(name, csvText, options = {}) {
  const result = loadDataset(name, csvText, options);
  return {
    dataset: result.dataset,
    rows: result.dataset.count(),
    problems: countProblems(result.validation),
    columns: result.dataset.columns(),
    key: keyColumns(result.dataset.schema),
    checksum: checksumOfDataset(result.dataset),
  };
}

export function checksumOfDataset(dataset) {
  const rows = dataset.all();
  const parts = [dataset.schema.name, rows.length];
  for (const row of rows) {
    parts.push(dataset.keyOf(row));
    for (const column of dataset.moneyColumns()) parts.push(row[column] ?? 0);
  }
  return parts.join('|');
}

export function preview(csvText, options = {}) {
  const schema = schemaOf(options.dataset ?? 'items');
  const parsed = parseCsv(csvText);
  const renamed = renameColumns(parsed.rows, parsed.header, schema);
  const validation = validateRows(schema, renamed, { allowUnknown: true });
  return {
    header: parsed.header,
    rowCount: parsed.rows.length,
    validation,
    sample: renamed.slice(0, options.rows ?? 5),
  };
}

export function compatibleInput(name, header) {
  const schema = schemaOf(name);
  const declared = {
    name: 'input',
    columns: header.map((column) => ({ name: column, type: 'text' })),
  };
  return compareSchemas(schema, declared);
}

export function datasetNames() {
  return Object.keys(DATASETS).sort();
}

export function rejectUnknownColumns(name) {
  const schema = schemaOf(name);
  const allowed = new Set(schema.columns.map((column) => column.name));
  return (row) => Object.keys(row).some((column) => !allowed.has(column));
}
