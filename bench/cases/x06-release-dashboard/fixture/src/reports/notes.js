import { groupChanges, breakingChanges, changeSummary, normalizeChanges } from '../domain/changes.js';
import { titleCase, singleLine, bulletList, truncate } from '../util/text.js';

export const NOTE_SECTIONS = ['breaking', 'feature', 'fix', 'security', 'chore'];

export function sectionFor(kind) {
  return NOTE_SECTIONS.includes(kind) ? kind : 'chore';
}

export function releaseNotes(release, options = {}) {
  const changes = normalizeChanges(release?.changelog ?? []);
  const groups = groupChanges(changes);
  const sections = NOTE_SECTIONS.filter((kind) => (groups[kind] ?? []).length > 0).map((kind) => ({
    kind,
    title: titleCase(kind),
    lines: (groups[kind] ?? []).map((change) => change.summary),
  }));
  return {
    title: `${release?.service ?? 'service'} ${release?.version ?? '0.0.0'}`,
    environment: release?.environment ?? 'development',
    sections,
    summary: changeSummary(changes),
    breaking: breakingChanges(changes).length > 0,
    generated_at: options.at ?? new Date().toISOString(),
  };
}

export function notesMarkdown(notes, options = {}) {
  const lines = [`# ${notes.title}`, '', `Environment: ${notes.environment}`, ''];
  for (const section of notes.sections) {
    lines.push(`## ${section.title}`, '');
    lines.push(bulletList(section.lines));
    lines.push('');
  }
  if (options.footer) lines.push(options.footer);
  return lines.join('\n').trimEnd();
}

export function notesText(notes, options = {}) {
  const lines = [`${notes.title} (${notes.environment})`];
  for (const section of notes.sections) {
    lines.push('', section.title.toUpperCase());
    for (const entry of section.lines) lines.push(`  - ${truncate(entry, 160)}`);
  }
  if (options.footer) lines.push('', singleLine(options.footer));
  return lines.join('\n');
}

export function notesSummary(notes) {
  return {
    title: notes.title,
    sections: notes.sections.length,
    changes: notes.summary.changes,
    breaking: notes.breaking,
  };
}

export function breakingNotices(notes) {
  const section = notes.sections.find((entry) => entry.kind === 'breaking');
  return section === undefined ? [] : [...section.lines];
}

export function mergeNotes(notesList) {
  const sections = new Map();
  for (const notes of notesList ?? []) {
    for (const section of notes.sections) {
      const bucket = sections.get(section.kind) ?? [];
      bucket.push(...section.lines);
      sections.set(section.kind, bucket);
    }
  }
  return [...sections.entries()]
    .map(([kind, lines]) => ({ kind, title: titleCase(kind), lines }))
    .sort((left, right) => NOTE_SECTIONS.indexOf(left.kind) - NOTE_SECTIONS.indexOf(right.kind));
}
