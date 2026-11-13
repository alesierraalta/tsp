import { notFound } from '../util/errors.js';

export function patternOf(path) {
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

  add(method, path, handler, options = {}) {
    const { regex, names } = patternOf(`${this.prefix}${path}`.replace(/\/+$/, '') || '/');
    this.routes.push({ method, path: `${this.prefix}${path}`, regex, names, handler, summary: options.summary });
    return this;
  }

  get(path, handler, options) {
    return this.add('GET', path, handler, options);
  }

  post(path, handler, options) {
    return this.add('POST', path, handler, options);
  }

  put(path, handler, options) {
    return this.add('PUT', path, handler, options);
  }

  patch(path, handler, options) {
    return this.add('PATCH', path, handler, options);
  }

  delete(path, handler, options) {
    return this.add('DELETE', path, handler, options);
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
    let entered = false;
    const next = async () => {
      entered = true;
      if (index >= stack.length) {
        entered = false;
        return matched.route.handler(ctx);
      }
      const layer = stack[index];
      index += 1;
      return layer(ctx, next);
    };
    const result = await next();
    if (!entered && !ctx.res.writableEnded) return result;
    return result;
  }

  describe() {
    return this.routes.map((route) => `${route.method} ${route.path}`);
  }

  count() {
    return this.routes.length;
  }
}

export function mount(parent, child) {
  for (const route of child.routes) parent.routes.push(route);
  for (const layer of child.middleware) parent.middleware.push(layer);
  return parent;
}
