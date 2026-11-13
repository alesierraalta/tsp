const BLOCK_RE = /\{\{#(\w+)([^}]*)\}\}([\s\S]*?)\{\{\/\1\}\}/;
const VARIABLE_RE = /\{\{(\{)?\s*([\w.@]+(?:\s+[\w.@]+)*)\s*\}?\}\}/;

export function parseTemplate(source) {
  return { source: String(source ?? ''), nodes: parseNodes(String(source ?? '')) };
}

function parseNodes(source) {
  const nodes = [];
  let rest = source;
  while (rest.length > 0) {
    const block = BLOCK_RE.exec(rest);
    const variable = VARIABLE_RE.exec(rest);
    if (block && (!variable || block.index <= variable.index)) {
      const before = rest.slice(0, block.index);
      if (before) nodes.push(...literalNodes(before));
      const [whole, name, rawArgs, body] = block;
      nodes.push({ kind: 'block', name, args: parseArgs(rawArgs), body: parseNodes(body) });
      rest = rest.slice(block.index + whole.length);
      continue;
    }
    if (variable) {
      const before = rest.slice(0, variable.index);
      if (before) nodes.push(...literalNodes(before));
      const [whole, triple, expression] = variable;
      const [head, ...tail] = expression.split(/\s+/);
      nodes.push(
        tail.length === 0
          ? { kind: 'variable', path: head, escape: triple !== '{' }
          : { kind: 'variable', helper: head, path: tail[0], escape: triple !== '{' },
      );
      rest = rest.slice(variable.index + whole.length);
      continue;
    }
    nodes.push(...literalNodes(rest));
    break;
  }
  return nodes;
}

function literalNodes(text) {
  if (text === '') return [];
  return [{ kind: 'text', value: text }];
}

function parseArgs(raw) {
  const text = String(raw ?? '').trim();
  if (text === '') return { path: '', literal: undefined };
  const quoted = /^"([^"]*)"$/.exec(text);
  if (quoted) return { path: '', literal: quoted[1] };
  const named = /^(\w+)\s*=\s*"([^"]*)"$/.exec(text);
  if (named) return { path: named[1], literal: named[2] };
  return { path: text, literal: undefined };
}

export function lookup(scope, path) {
  if (path === '.') return scope;
  const parts = String(path).split('.');
  let value = scope;
  for (const part of parts) {
    if (value === null || value === undefined) return undefined;
    if (part === 'this') continue;
    value = value[part];
  }
  return value;
}

export function renderTemplate(template, scope, helpers = {}) {
  const nodes = template.nodes ?? parseNodes(String(template ?? ''));
  return renderNodes(nodes, scope, helpers);
}

function renderNodes(nodes, scope, helpers, out = []) {
  for (const node of nodes) {
    if (node.kind === 'text') {
      out.push(node.value);
      continue;
    }
    if (node.kind === 'variable') {
      const value = lookup(scope, node.path);
      const render = (nodes, nextScope) => renderNodes(nodes, nextScope ?? scope, helpers);
      if (node.helper !== undefined) {
        const helper = helpers[node.helper];
        if (typeof helper !== 'function') {
          out.push('');
          continue;
        }
        out.push(helper(value, scope, render));
        continue;
      }
      const text = value === undefined || value === null ? '' : String(value);
      out.push(node.escape ? (helpers.escape ? helpers.escape(text) : text) : text);
      continue;
    }
    if (node.kind === 'block') {
      const value = node.args.literal ?? lookup(scope, node.args.path);
      const helper = helpers[node.name];
      if (typeof helper === 'function') {
        out.push(helper(value, scope, (inner, nextScope) => renderNodes(inner, nextScope ?? scope, helpers)));
        continue;
      }
      if (node.name === 'each' && Array.isArray(value)) {
        for (const item of value) renderNodes(node.body, item, helpers, out);
        continue;
      }
      if (node.name === 'if') {
        if (value) renderNodes(node.body, scope, helpers, out);
        continue;
      }
      if (node.name === 'unless') {
        if (!value) renderNodes(node.body, scope, helpers, out);
      }
    }
  }
  return out.join('');
}

export function countNodes(nodes) {
  let total = 0;
  for (const node of nodes ?? []) {
    total += 1;
    if (node.kind === 'block') total += countNodes(node.body);
  }
  return total;
}

export function variablesOf(nodes, out = new Set()) {
  for (const node of nodes ?? []) {
    if (node.kind === 'variable') out.add(node.path);
    if (node.kind === 'block') {
      if (node.args?.path) out.add(node.args.path);
      variablesOf(node.body, out);
    }
  }
  return out;
}

const COMMENT_RE = /\{\{!--[\s\S]*?--\}\}/g;

export function stripComments(source) {
  return String(source ?? '').replace(COMMENT_RE, '');
}

export function renderList(items, render) {
  return (items ?? []).map((item, index) => render(item, index)).join('');
}

export function compileWithPartials(source, partials) {
  let out = String(source ?? '');
  for (const [name, body] of Object.entries(partials ?? {})) {
    out = out.split(`{{> ${name}}}`).join(body);
  }
  return out;
}

export function templateReport(template) {
  const nodes = template.nodes ?? [];
  const variables = [...variablesOf(nodes)].sort();
  return {
    name: template.name ?? 'inline',
    nodes: countNodes(nodes),
    variables,
    blocks: nodes.filter((node) => node.kind === 'block').map((node) => node.name),
  };
}

export function inlinePartials(nodes) {
  const out = [];
  for (const node of nodes ?? []) {
    if (node.kind === 'block' && node.name === 'partial') out.push(node.args.literal ?? node.args.path);
    if (node.kind === 'block') out.push(...inlinePartials(node.body));
  }
  return out;
}

export function assertBalanced(source) {
  const text = stripComments(source);
  const opens = (text.match(/\{\{#/g) ?? []).length;
  const closes = (text.match(/\{\{\//g) ?? []).length;
  if (opens !== closes) throw new TypeError(`template has ${opens} block(s) open and ${closes} close(s)`);
  return true;
}
