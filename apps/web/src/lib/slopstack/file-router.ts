import { Component, createElement, Suspense, type ReactNode } from 'react';
import type { ComponentType } from 'react';
import type { ViewContext } from './types';
type ComponentLike<P = Record<string, never>> = ComponentType<P>;

export type FileRouteModule = {
  default?: ComponentLike<{ context: ViewContext; children?: ReactNode }>;
  loading?: ComponentLike<{ context: ViewContext }>;
  error?: ComponentLike<{ context: ViewContext; error: Error; reset: () => void }>;
  notFound?: ComponentLike<{ context: ViewContext }>;
};

export type FileRoute = {
  id: string;
  directory: string;
  path: string;
  segments: readonly string[];
  module: FileRouteModule;
  layouts: readonly ComponentLike<{ context: ViewContext; children?: ReactNode }>[];
};

export type FileRouteMatch = { route: FileRoute; params: Record<string, string> };

/** Build routes from eager modules collected under `routes/`. Special files are
 * colocated as `layout`, `loading`, `error`, and `not-found` modules. */
export function createFileRoutes(modules: Readonly<Record<string, FileRouteModule>>): FileRoute[] {
  const files = Object.entries(modules);
  const routes = files
    .filter(([file, module]) => /\/page\.[jt]sx?$/.test(file) && module.default)
    .map(([file, module]) => {
      const routePath = file.replace(/^.*?routes\//, '').replace(/(^|\/)page\.[jt]sx?$/, '');
      const raw = routePath.split('/').filter(Boolean);
      const segments = raw.filter((segment) => !/^\(.*\)$/.test(segment));
      const path = `/${segments.map((segment) => segment.startsWith('[') && segment.endsWith(']')
        ? segment.startsWith('[...') ? '*' : `:${segment.slice(1, -1)}`
        : segment).join('/')}`;
      const layouts = files
        .filter(([layoutFile, layoutModule]) => /\/layout\.[jt]sx?$/.test(layoutFile)
          && layoutModule.default
          && file.startsWith(layoutFile.slice(0, layoutFile.lastIndexOf('/') + 1)))
        .sort(([a], [b]) => a.length - b.length)
        .map(([, layoutModule]) => layoutModule.default!);
      return { id: routePath || 'index', directory: file.slice(0, file.lastIndexOf('/') + 1), path, segments, module, layouts };
    })
    .sort((a, b) => (a.segments.length === 0 ? -1 : b.segments.length === 0 ? 1 : 0)
      || routeRank(b) - routeRank(a) || a.path.localeCompare(b.path));
  validateFileRoutes(routes);
  return routes;
}

/** Fail early on ambiguous or invalid route definitions while developing/building. */
export function validateFileRoutes(routes: readonly FileRoute[]) {
  const seen = new Map<string, string>();
  for (const route of routes) {
    const dynamicNames = new Set<string>();
    route.segments.forEach((segment, index) => {
      const catchAll = segment.startsWith('[...') && segment.endsWith(']');
      const dynamic = segment.startsWith('[') && segment.endsWith(']');
      if (catchAll && index !== route.segments.length - 1) throw new Error(`Catch-all route segment in "${route.id}" must be last.`);
      if (dynamic) {
        const name = catchAll ? segment.slice(4, -1) : segment.slice(1, -1);
        if (!/^[A-Za-z][A-Za-z0-9_]*$/.test(name)) throw new Error(`Invalid route parameter "${segment}" in "${route.id}".`);
        if (dynamicNames.has(name)) throw new Error(`Duplicate route parameter "${name}" in "${route.id}".`);
        dynamicNames.add(name);
      } else if (segment.startsWith('[') || segment.endsWith(']')) {
        throw new Error(`Invalid route segment "${segment}" in "${route.id}".`);
      }
    });
    const signature = route.segments.map((segment) => segment.startsWith('[...') ? '*' : segment.startsWith('[') ? ':' : segment).join('/');
    const previous = seen.get(signature);
    if (previous) throw new Error(`Routes "${previous}" and "${route.id}" resolve to the same URL pattern.`);
    seen.set(signature, route.id);
  }
}

function routeRank(route: FileRoute) {
  return route.segments.reduce((rank, segment) => rank + (segment.startsWith('[') ? 1 : 10), 0);
}

export function matchFileRoute(routes: readonly FileRoute[], pathname: string): FileRouteMatch | undefined {
  const target = pathname.split('/').filter(Boolean);
  for (const route of routes) {
    const params: Record<string, string> = {};
    let matched = true;
    for (let index = 0; index < route.segments.length; index += 1) {
      const part = route.segments[index];
      if (part.startsWith('[...')) {
        const tail = target.slice(index);
        if (!tail.length) { matched = false; break; }
        params[part.slice(4, -1)] = tail.map(decode).join('/');
        break;
      }
      if (target[index] === undefined) { matched = false; break; }
      if (part.startsWith('[') && part.endsWith(']')) params[part.slice(1, -1)] = decode(target[index]);
      else if (part !== target[index]) { matched = false; break; }
    }
    if (matched && (route.segments.some((part) => part.startsWith('[...')) || target.length === route.segments.length)) return { route, params };
  }
}

export type FileRouteResolution =
  | { status: 'rendered'; match: FileRouteMatch; context: ViewContext; element: ReactNode; loading?: ReactNode; error?: FileRouteModule['error'] }
  | { status: 'not-found'; element?: ReactNode };

/** Resolve and compose page and nested layouts. The caller can place `loading`
 * in a Suspense boundary and `error` in an error boundary. */
export function resolveFileRoute(
  routes: readonly FileRoute[],
  modules: Readonly<Record<string, FileRouteModule>>,
  input: { pathname: string; search?: URLSearchParams; navigate: ViewContext['navigate'] },
): FileRouteResolution {
  const match = matchFileRoute(routes, input.pathname);
  const context: ViewContext = { params: match?.params ?? {}, search: input.search ?? new URLSearchParams(), navigate: input.navigate };
  if (!match) {
    const notFound = Object.entries(modules).find(([file]) => /\/not-found\.[jt]sx?$/.test(file))?.[1].default;
    return { status: 'not-found', element: notFound ? createElement(notFound, { context }) : undefined };
  }
  let element: ReactNode = createElement(match.route.module.default!, { context });
  for (const Layout of [...match.route.layouts].reverse()) element = createElement(Layout, { context, children: element });
  const routeFiles = [match.route.directory, ...parentDirectories(match.route.directory)];
  const moduleFor = (kind: 'loading' | 'error') => routeFiles
    .map((directory) => Object.entries(modules).find(([file]) => file.startsWith(directory) && file.slice(directory.length).split('/').length === 1 && new RegExp(`/${kind}\\.[jt]sx?$`).test(file))?.[1].default)
    .find(Boolean);
  const loadingModule = moduleFor('loading');
  const errorModule = moduleFor('error');
  return {
    status: 'rendered', match, context, element,
    loading: loadingModule ? createElement(loadingModule, { context }) : undefined,
    error: errorModule as FileRouteModule['error'],
  };
}

/** Render a resolved route with its colocated Suspense and error fallbacks. */
export function FileRouteOutlet({ resolution }: { resolution: FileRouteResolution }) {
  if (resolution.status === 'not-found') return resolution.element ?? null;
  return createElement(RouteErrorBoundary, {
    key: resolution.match.route.id,
    fallback: resolution.error,
    context: resolution.context,
    children: createElement(Suspense, { fallback: resolution.loading ?? null, children: resolution.element }),
  });
}

class RouteErrorBoundary extends Component<{
  fallback?: FileRouteModule['error'];
  context: ViewContext;
  children?: ReactNode;
}, { error?: Error }> {
  state: { error?: Error } = {};

  static getDerivedStateFromError(error: Error) { return { error }; }
  render() {
    if (this.state.error && this.props.fallback) {
      return createElement(this.props.fallback, {
        context: this.props.context,
        error: this.state.error,
        reset: () => this.setState({ error: undefined }),
      });
    }
    if (this.state.error) return createElement('div', { role: 'alert' }, 'Something went wrong.');
    return this.props.children;
  }
}

function parentDirectories(directory: string) {
  const result: string[] = [];
  let current = directory.replace(/[^/]+\/$/, '');
  while (current.includes('/routes/')) {
    const parent = current.slice(0, current.lastIndexOf('/', current.length - 2) + 1);
    if (!parent || parent === current) break;
    result.push(parent);
    current = parent;
  }
  return result;
}

function decode(value: string) {
  try { return decodeURIComponent(value); } catch { return value; }
}
