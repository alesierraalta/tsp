import { resolve } from 'node:path';

export function configFromEnv(env = process.env) {
  const root = env.HUB_ROOT ? resolve(env.HUB_ROOT) : process.cwd();
  return {
    root,
    port: Number(env.PORT ?? 0),
    secret: env.HUB_SECRET ?? 'development-secret',
    dataRoot: env.DATA_ROOT ? resolve(env.DATA_ROOT) : resolve(root, 'var', 'data'),
    mediaRoot: env.MEDIA_ROOT ? resolve(env.MEDIA_ROOT) : resolve(root, 'var', 'media'),
    templateRoot: env.TEMPLATE_ROOT ? resolve(env.TEMPLATE_ROOT) : resolve(root, 'var', 'templates'),
    pluginRoot: env.PLUGIN_ROOT ? resolve(env.PLUGIN_ROOT) : resolve(root, 'var', 'plugins'),
    exportRoot: env.EXPORT_ROOT ? resolve(env.EXPORT_ROOT) : resolve(root, 'var', 'exports'),
    logLevel: env.LOG_LEVEL ?? 'info',
    maxBodyBytes: Number(env.MAX_BODY_BYTES ?? 512 * 1024),
  };
}

export function describeConfig(config) {
  return {
    root: config.root,
    port: config.port,
    dataRoot: config.dataRoot,
    mediaRoot: config.mediaRoot,
    templateRoot: config.templateRoot,
    pluginRoot: config.pluginRoot,
    exportRoot: config.exportRoot,
    maxBodyBytes: config.maxBodyBytes,
  };
}

export function validateConfig(config) {
  const problems = [];
  if (typeof config.secret !== 'string' || config.secret.length < 8) problems.push('secret is shorter than 8 characters');
  if (!Number.isInteger(config.port) || config.port < 0 || config.port > 65535) problems.push('port is out of range');
  for (const key of ['dataRoot', 'mediaRoot', 'templateRoot', 'pluginRoot', 'exportRoot']) {
    if (typeof config[key] !== 'string' || config[key] === '') problems.push(`${key} is not set`);
  }
  if (problems.length > 0) throw new TypeError(problems.join('; '));
  return config;
}
