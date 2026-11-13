import { startApp } from './server.js';
import { configFromEnv, describeConfig, validateConfig } from './config.js';
import { createLogger } from './util/log.js';

const config = validateConfig(configFromEnv());
const logger = createLogger({ level: config.logLevel });
const started = await startApp({ ...config, logger });

logger.info('content hub listening', { ...describeConfig(config), url: started.url });

const shutdown = async (signal) => {
  logger.info('shutting down', { signal });
  await started.close();
  process.exit(0);
};

process.on('SIGINT', () => void shutdown('SIGINT'));
process.on('SIGTERM', () => void shutdown('SIGTERM'));
