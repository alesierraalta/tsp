const VERSION = /^v?(\d{1,6})\.(\d{1,6})\.(\d{1,6})(?:-([0-9A-Za-z.-]{1,32}))?$/;

export function parseVersion(value) {
  const match = VERSION.exec(String(value ?? '').trim());
  if (!match) throw new TypeError(`not a semantic version: ${value}`);
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    prerelease: match[4] ?? null,
  };
}

export function isVersion(value) {
  return VERSION.test(String(value ?? '').trim());
}

export function formatVersion(parts) {
  const base = `${parts.major}.${parts.minor}.${parts.patch}`;
  return parts.prerelease ? `${base}-${parts.prerelease}` : base;
}

export function isPrerelease(value) {
  return parseVersion(value).prerelease !== null;
}

export function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  for (const field of ['major', 'minor', 'patch']) {
    if (a[field] !== b[field]) return a[field] - b[field];
  }
  if (a.prerelease === b.prerelease) return 0;
  if (a.prerelease === null) return 1;
  if (b.prerelease === null) return -1;
  return a.prerelease < b.prerelease ? -1 : 1;
}

export function bumpVersion(value, kind = 'patch') {
  const parts = parseVersion(value);
  if (kind === 'major') return formatVersion({ major: parts.major + 1, minor: 0, patch: 0, prerelease: null });
  if (kind === 'minor') return formatVersion({ major: parts.major, minor: parts.minor + 1, patch: 0, prerelease: null });
  if (kind === 'patch') return formatVersion({ ...parts, patch: parts.patch + 1, prerelease: null });
  if (kind === 'prerelease') return formatVersion({ ...parts, prerelease: parts.prerelease ?? 'rc.1' });
  throw new TypeError(`unknown bump ${kind}`);
}

export function channelOf(value) {
  const parts = parseVersion(value);
  if (parts.prerelease !== null) return /rc/.test(parts.prerelease) ? 'candidate' : 'beta';
  if (parts.major === 0) return 'alpha';
  return 'stable';
}

export function sortVersions(versions, direction = 'desc') {
  const sign = direction === 'desc' ? -1 : 1;
  return [...(versions ?? [])].sort((left, right) => sign * compareVersions(left, right));
}

export function latestOf(versions) {
  return sortVersions(versions, 'desc')[0];
}

export function versionRows(versions) {
  return (versions ?? []).map((version) => ({ version, channel: channelOf(version), prerelease: isPrerelease(version) }));
}

export function nextVersion(versions, kind = 'patch') {
  const latest = latestOf(versions);
  return latest === undefined ? '0.1.0' : bumpVersion(latest, kind);
}
