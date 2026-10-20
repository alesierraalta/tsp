import { readSettings, describeSettings } from '../src/settings.js';

const settingNames = [
  'SEARCH_MAX_QUERY_LENGTH',
  'SEARCH_PAGE_SIZE',
  'SEARCH_SORT',
];

export function renderEnvironment(input) {
  const output = {};
  for (const name of settingNames) {
    if (input[name] !== undefined) {
      if (typeof input[name] !== 'string') {
        throw new TypeError(`${name} must be text`);
      }
      output[name] = input[name];
    }
  }
  readSettings(output);
  return Object.freeze(output);
}

export function renderManifest(input, service = 'search') {
  if (!/^[a-z][a-z0-9-]*$/.test(service)) {
    throw new Error('invalid service name');
  }
  const environment = renderEnvironment(input);
  return {
    service,
    environment,
    settings: describeSettings(readSettings(environment)),
  };
}

export function environmentEntries(input) {
  return Object.entries(renderEnvironment(input))
    .sort(([a], [b]) => a.localeCompare(b));
}

export function serializeEnvironment(input) {
  return JSON.stringify(Object.fromEntries(environmentEntries(input)), null, 2);
}
