import { startApp } from './server.js';
import { createLogger } from './util/log.js';

const port = Number(process.env.PORT ?? 8080);
const secret = process.env.SETTINGS_SECRET ?? 'development-secret';
const logger = createLogger({ level: process.env.LOG_LEVEL ?? 'info' });

const started = await startApp({
  port,
  secret,
  logger,
  dataRoot: process.env.DATA_ROOT,
  archiveRoot: process.env.ARCHIVE_ROOT,
  documentRoot: process.env.DOCUMENT_ROOT,
  snapshotDir: process.env.SNAPSHOT_DIR,
});

started.app.router.get('/ready', async (ctx) => {
  ctx.res.statusCode = 200;
  ctx.res.end('ready');
});

logger.info('tenant settings gateway listening', { url: started.url });

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    logger.info('shutting down', { signal });
    started.close().then(() => process.exit(0), () => process.exit(1));
  });
}
