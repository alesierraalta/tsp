import { loadConfig } from './config.js';
import { importDataset } from './etl/loader.js';
import { exportInsert, exportSelect, exportSearch } from './export/sql.js';
import { toCsv, toTsv } from './export/csv.js';
import { describeManifest, manifestFor } from './export/manifest.js';
import { summarize } from './reports/summary.js';
import { statementFor } from './reports/statement.js';
import { renderPage } from './render/pages/index.js';
import { estimateTotals } from './reports/estimate.js';

export function createPipeline(options = {}) {
  const config = loadConfig(options.config ?? {});
  const datasets = new Map();
  const history = [];

  function importCsv(name, csvText, meta = {}) {
    const result = importDataset(name, csvText, { limit: options.limit });
    datasets.set(name, result.dataset);
    history.push({
      dataset: name,
      rows: result.rows,
      checksum: result.checksum,
      when: new Date(options.now ?? Date.now()).toISOString(),
      note: meta.note,
    });
    return result;
  }

  function datasetOf(name) {
    const dataset = datasets.get(name);
    if (!dataset) throw new Error(`dataset ${name} has not been imported`);
    return dataset;
  }

  function exportSql(name, options2 = {}) {
    const dataset = datasetOf(name);
    if (options2.search) {
      return exportSearch(dataset, options2.search, { column: options2.column, limit: options2.limit });
    }
    return exportSelect(dataset, options2);
  }

  function exportInserts(name) {
    return exportInsert(datasetOf(name));
  }

  function exportTable(name, format = 'csv') {
    const dataset = datasetOf(name);
    const rows = dataset.all();
    if (format === 'tsv') return toTsv(rows, dataset.columns());
    if (format === 'csv') return toCsv(rows, dataset.columns());
    throw new Error(`unknown table format: ${format}`);
  }

  function manifest(name, format = 'csv') {
    return manifestFor(datasetOf(name), { format, now: options.now });
  }

  function summary(name) {
    return summarize(datasetOf(name));
  }

  function statement(name) {
    return statementFor(datasetOf(name), { currency: config.currency });
  }

  function page(name, model) {
    return renderPage(name, { ...model, currency: config.currency }, config);
  }

  function estimate(name, options2 = {}) {
    return estimateTotals(datasetOf(name), options2);
  }

  return {
    config,
    history,
    datasets,
    importCsv,
    datasetOf,
    exportSql,
    exportInserts,
    exportTable,
    manifest,
    summary,
    statement,
    page,
    estimate,
    describe(name) {
      return describeManifest(manifest(name));
    },
    names() {
      return [...datasets.keys()].sort();
    },
  };
}

export const SAMPLE_ITEMS = [
  'sku,title,category,amount,stock,updated_at',
  'SKU-1,Desk lamp,lighting,120.50,4,2026-01-05',
  'SKU-2,Chair,seating,89.00,12,2026-01-06',
].join('\n');
