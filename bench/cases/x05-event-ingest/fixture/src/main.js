import { startApp } from './server.js';
import { configFromEnv, describeConfig, validateConfig } from './config.js';
import { createLogger } from './util/log.js';

const config = validateConfig(configFromEnv());
const logger = createLogger({ level: config.logLevel });
const started = await startApp({ ...config, logger });

logger.info('ingest listening', { ...describeConfig(config), url: started.url });

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, async () => {
    logger.info('shutting down', { signal });
    await started.close();
    process.exit(0);
  });
}
