import { resolve } from 'node:path';

export function configFromEnv(env = process.env) {
  return {
    root: env.DASHBOARD_ROOT ? resolve(env.DASHBOARD_ROOT) : process.cwd(),
    port: Number(env.PORT ?? 0),
    maxBodyBytes: Number(env.MAX_BODY_BYTES ?? 512 * 1024),
    defaultTenant: env.DEFAULT_TENANT ?? 'acme',
    logLevel: env.LOG_LEVEL ?? 'info',
    allowedHosts: (env.DASHBOARD_ALLOWED_HOSTS ?? '')
      .split(',')
      .map((host) => host.trim())
      .filter(Boolean),
    webhookSecret: env.WEBHOOK_SECRET ?? 'development-webhook-secret',
    retentionDays: Number(env.RETENTION_DAYS ?? 90),
    dashboardTitle: env.DASHBOARD_TITLE ?? 'Release dashboard',
  };
}

export function validateConfig(config) {
  const problems = [];
  if (!Number.isInteger(config.port) || config.port < 0 || config.port > 65535) problems.push('port is out of range');
  if (!Number.isFinite(config.maxBodyBytes) || config.maxBodyBytes < 1024) problems.push('maxBodyBytes is too small');
  if (!Number.isFinite(config.retentionDays) || config.retentionDays < 1) problems.push('retentionDays must be at least 1');
  if (config.webhookSecret.length < 8) problems.push('webhookSecret is too short');
  if (problems.length > 0) throw new TypeError(problems.join('; '));
  return config;
}

export function describeConfig(config) {
  return {
    root: config.root,
    port: config.port,
    maxBodyBytes: config.maxBodyBytes,
    defaultTenant: config.defaultTenant,
    allowedHosts: [...config.allowedHosts],
    title: config.dashboardTitle,
  };
}

export function redactedConfig(config) {
  return { ...describeConfig(config), webhookSecret: '***' };
}

export function allowedHostsOf(config) {
  return [...(config.allowedHosts ?? [])];
}
