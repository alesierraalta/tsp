import { notFound } from '../util/errors.js';

function patternOf(path) {
  const names = [];
  const source = String(path)
    .split('/')
    .map((part) => {
      if (part.startsWith(':')) {
        names.push(part.slice(1));
        return '([^/]+)';
      }
      return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    })
    .join('/');
  return { regex: new RegExp(`^${source}/?$`), names };
}

export class Router {
  constructor(options = {}) {
    this.routes = [];
    this.middleware = [];
    this.prefix = options.prefix ?? '';
  }

  use(fn) {
    this.middleware.push(fn);
    return this;
  }

  add(method, path, handler) {
    const { regex, names } = patternOf(`${this.prefix}${path}`.replace(/\/+$/, '') || '/');
    this.routes.push({ method, path: `${this.prefix}${path}`, regex, names, handler });
    return this;
  }

  get(path, handler) {
    return this.add('GET', path, handler);
  }

  post(path, handler) {
    return this.add('POST', path, handler);
  }

  put(path, handler) {
    return this.add('PUT', path, handler);
  }

  patch(path, handler) {
    return this.add('PATCH', path, handler);
  }

  delete(path, handler) {
    return this.add('DELETE', path, handler);
  }

  match(method, path) {
    const wanted = String(method).toUpperCase();
    for (const route of this.routes) {
      if (route.method !== wanted) continue;
      const found = route.regex.exec(path);
      if (!found) continue;
      const params = {};
      route.names.forEach((name, index) => {
        params[name] = decodeURIComponent(found[index + 1] ?? '');
      });
      return { route, params };
    }
    return undefined;
  }

  async handle(ctx) {
    const matched = this.match(ctx.method, ctx.path);
    if (!matched) throw notFound(`route ${ctx.method} ${ctx.path}`);
    ctx.params = matched.params;
    const stack = [...this.middleware];
    let index = 0;
    const next = async () => {
      if (index >= stack.length) return matched.route.handler(ctx);
      const layer = stack[index];
      index += 1;
      return layer(ctx, next);
    };
    return next();
  }

  describe() {
    return this.routes.map((route) => `${route.method} ${route.path}`);
  }
}

export function mount(parent, child) {
  for (const route of child.routes) parent.routes.push(route);
  return parent;
}
