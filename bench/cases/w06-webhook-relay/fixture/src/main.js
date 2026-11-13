import { createRelay, sampleDelivery } from './relay.js';
import { HookRegistry } from './jobs/registry.js';
import { hookSummary } from './hooks/runner.js';
import { createLogger } from './util/log.js';
import { isRelayError } from './util/errors.js';
import { fileURLToPath } from 'node:url';

const logger = createLogger({ level: process.env.LOG_LEVEL ?? 'info' });
const hooks = new HookRegistry([
  {
    name: 'tag-import',
    event: 'import.finished',
    script: fileURLToPath(new URL('../hooks/tag.mjs', import.meta.url)),
  },
]);
const relay = createRelay({
  logger,
  hooks,
  transport: async () => ({ status: 200 }),
});

const row = relay.enqueue(sampleDelivery({ payload: { id: 'ord_1', total: 1999 } }));
logger.info('queued', { id: row.id, due: relay.stats().due });

try {
  const results = await relay.drain(2);
  logger.info('dispatched', { results });
  const hook = await relay.hookForImport('payload.json');
  logger.info('hook', hookSummary(hook));
  logger.info('stats', relay.stats().counts);
} catch (error) {
  if (isRelayError(error)) {
    logger.error(error.message, { code: error.code });
    process.exitCode = 1;
  } else {
    throw error;
  }
}
