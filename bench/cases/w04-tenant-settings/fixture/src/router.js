import { notFound } from './http/errors.js';

function segments(path) {
  return path.split('/').filter((part) => part.length > 0);
}

function compile(pattern) {
  const parts = segments(pattern);
  const names = [];
  const matchers = parts.map((part) => {
    if (part.startsWith(':')) {
      names.push(part.slice(1));
      return '([^/]+)';
    }
    if (part === '*') {
      names.push('rest');
      return '(.*)';
    }
    return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  });
  return {
    names,
    regex: new RegExp(`^/${matchers.join('/')}${parts.length === 0 ? '?$' : '/?$'}`),
  };
}

export class Router {
  constructor(options = {}) {
    this.routes = [];
    this.middleware = [...(options.middleware ?? [])];
    this.notFoundHandler = options.notFound ?? (() => {
      throw notFound('route');
    });
  }

  use(fn) {
    if (typeof fn !== 'function') throw new TypeError('middleware must be a function');
    this.middleware.push(fn);
    return this;
  }

  add(method, pattern, handler) {
    if (typeof handler !== 'function') throw new TypeError('a route handler is required');
    const { names, regex } = compile(pattern);
    this.routes.push({ method: method.toUpperCase(), pattern, handler, names, regex });
    return this;
  }

  get(pattern, handler) {
    return this.add('GET', pattern, handler);
  }

  post(pattern, handler) {
    return this.add('POST', pattern, handler);
  }

  put(pattern, handler) {
    return this.add('PUT', pattern, handler);
  }

  patch(pattern, handler) {
    return this.add('PATCH', pattern, handler);
  }

  delete(pattern, handler) {
    return this.add('DELETE', pattern, handler);
  }

  match(method, path) {
    const wanted = method.toUpperCase();
    let pathMatched = false;
    for (const route of this.routes) {
      const match = route.regex.exec(path);
      if (!match) continue;
      pathMatched = true;
      if (route.method !== wanted) continue;
      const params = {};
      route.names.forEach((name, index) => {
        params[name] = decodeURIComponent(match[index + 1]);
      });
      return { route, params };
    }
    if (pathMatched) return { methodMismatch: true };
    return undefined;
  }

  async handle(ctx) {
    const head = ctx.method === 'HEAD' ? 'GET' : ctx.method;
    const found = this.match(head, ctx.path);
    if (!found) {
      return this.notFoundHandler(ctx);
    }
    if (found.methodMismatch) {
      return this.notFoundHandler(ctx);
    }
    ctx.params = found.params;
    ctx.route = found.route;
    let index = -1;
    const chain = [...this.middleware];
    const next = async () => {
      index += 1;
      if (index < chain.length) {
        return chain[index](ctx, next);
      }
      return found.route.handler(ctx);
    };
    return next();
  }

  describe() {
    return this.routes.map((route) => `${route.method} ${route.pattern}`);
  }
}
