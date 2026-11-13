import { resolve } from 'node:path';

export function configFromEnv(env = process.env) {
  const root = env.INGEST_ROOT ? resolve(env.INGEST_ROOT) : process.cwd();
  return {
    root,
    port: Number(env.PORT ?? 0),
    maxBodyBytes: Number(env.MAX_BODY_BYTES ?? 1024 * 1024),
    maxSamples: Number(env.MAX_SAMPLES ?? 100000),
    defaultPlan: env.DEFAULT_PLAN ?? 'growth',
    logLevel: env.LOG_LEVEL ?? 'info',
    region: env.REGION ?? 'eu-west-1',
  };
}

export function validateConfig(config) {
  const problems = [];
  if (!Number.isInteger(config.port) || config.port < 0 || config.port > 65535) problems.push('port is out of range');
  if (!Number.isFinite(config.maxBodyBytes) || config.maxBodyBytes < 1024) problems.push('maxBodyBytes is too small');
  if (!Number.isFinite(config.maxSamples) || config.maxSamples < 1) problems.push('maxSamples must be at least 1');
  if (!['free', 'growth', 'scale'].includes(config.defaultPlan)) problems.push('defaultPlan is unknown');
  if (problems.length > 0) throw new TypeError(problems.join('; '));
  return config;
}

export function describeConfig(config) {
  return {
    root: config.root,
    port: config.port,
    maxBodyBytes: config.maxBodyBytes,
    defaultPlan: config.defaultPlan,
    region: config.region,
  };
}

export function regionLatency(region) {
  const table = { 'eu-west-1': 12, 'us-east-1': 84, 'ap-south-1': 140 };
  return table[region] ?? 100;
}
