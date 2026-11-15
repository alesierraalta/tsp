import { createPipeline, SAMPLE_ITEMS } from './pipeline.js';
import { ImportError } from './util/errors.js';
import { createLogger } from './util/log.js';

const logger = createLogger({ level: process.env.LOG_LEVEL ?? 'info' });
const pipeline = createPipeline();

try {
  const result = pipeline.importCsv('items', SAMPLE_ITEMS);
  logger.info('imported', { rows: result.rows, checksum: result.checksum });
  const summary = pipeline.summary('items');
  logger.info('summary', { rows: summary.rows, totalCents: summary.totalCents });
  const csv = pipeline.exportTable('items', 'csv');
  process.stdout.write(csv);
} catch (error) {
  if (error instanceof ImportError) {
    logger.error(error.message, { code: error.code });
    process.exitCode = 1;
  } else {
    throw error;
  }
}
