import { EgressError } from '../util/errors.js';
import { parseUrl } from '../util/url.js';

export function probeFor(hostname) {
  return `http://${String(hostname ?? '')}/`;
}

export function isInternalHost(hostname, rules) {
  return rules.blocksInternal(probeFor(hostname));
}

export function assertPublicHost(hostname, rules) {
  if (isInternalHost(hostname, rules)) {
    throw new EgressError(`${hostname} is an internal address`);
  }
  return hostname;
}

export function assertScheme(url, rules) {
  if (!rules.allowsScheme(url.protocol)) {
    throw new EgressError(`scheme ${url.protocol} is not admitted`);
  }
  return url;
}

export function assertAllowlisted(url, rules) {
  if (!rules.allowsHost(url.hostname)) {
    throw new EgressError(`host ${url.hostname} is not on the allowlist`);
  }
  return url;
}

export function assertTarget(raw, rules) {
  let url;
  try {
    url = parseUrl(raw);
  } catch (error) {
    throw new EgressError(error.message);
  }
  assertScheme(url, rules);
  assertAllowlisted(url, rules);
  assertPublicHost(url.hostname, rules);
  return url;
}

export function planFor(raw, rules) {
  const url = assertTarget(raw, rules);
  return {
    url,
    href: url.href,
    host: url.hostname,
    port: url.port,
    path: url.pathname,
  };
}

export function describeTarget(raw, rules) {
  const url = assertTarget(raw, rules);
  return url.port === '' ? url.hostname : `${url.hostname}:${url.port}`;
}
