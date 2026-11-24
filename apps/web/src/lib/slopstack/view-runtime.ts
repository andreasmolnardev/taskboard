import { createElement, type ReactNode } from 'react';
import { canAccess, type AccessContext } from './access-runtime';
import type { ApplicationDefinition, NavigationItem, ShellDefinition, ViewDefinition, ViewContext } from './types';
import { renderLayout, renderShell } from './layout-runtime';

export type ResolvedView = { view: ViewDefinition; params: Readonly<Record<string, string>> };
export type AccessControlledView =
  | { status: 'allowed'; view: ViewDefinition; params: Readonly<Record<string, string>> }
  | { status: 'denied'; view: ViewDefinition; redirectTo?: string };

function segments(path: string) {
  return path.replace(/\/+$/, '').split('/').filter(Boolean);
}

/** Resolves exact and `:parameter` view paths without binding the app to a router package. */
export function resolveView(views: readonly ViewDefinition[], pathname: string): ResolvedView | undefined {
  const target = segments(pathname);
  for (const view of views) {
    const pattern = segments(view.path);
    const params: Record<string, string> = {};
    let matched = true;
    for (let index = 0; index < pattern.length; index += 1) {
      const token = pattern[index];
      if (token === '*') {
        params['*'] = target.slice(index).map(decodeSegment).join('/');
        break;
      }
      const segment = target[index];
      if (segment === undefined) { matched = false; break; }
      if (token.startsWith(':')) params[token.slice(1)] = decodeSegment(segment);
      else if (token !== segment) { matched = false; break; }
    }
    if (matched && (pattern.includes('*') || target.length === pattern.length)) return { view, params };
  }
}

/** Resolves a route and its client-side visibility rule. Server APIs must enforce the same permission. */
export function resolveAccessibleView(
  views: readonly ViewDefinition[],
  pathname: string,
  access: AccessContext,
  unauthorizedPath?: string,
): AccessControlledView | undefined {
  const resolved = resolveView(views, pathname);
  if (!resolved) return undefined;
  if (!canAccess(resolved.view.access, access)) {
    return { status: 'denied', view: resolved.view, redirectTo: unauthorizedPath };
  }
  return { status: 'allowed', ...resolved };
}

/** Filters shell navigation using the same client-side rules as views and actions. */
export function getAccessibleNavigation(shell: ShellDefinition, access: AccessContext): readonly NavigationItem[] {
  return shell.navigation.filter((item) => canAccess(item.access, access));
}

/** Produces the React element for a resolved view and its typed route context. */
export function renderView(view: ViewDefinition, context: ViewContext) {
  return createElement(view.component, { context });
}

export type ApplicationRouteResult =
  | { status: 'not-found' }
  | { status: 'denied'; view: ViewDefinition; redirectTo?: string }
  | { status: 'rendered'; view: ViewDefinition; params: Readonly<Record<string, string>>; element: ReactNode };

/** Composes route matching, client access, optional layouts, and the application shell. */
export function resolveApplicationRoute(
  application: ApplicationDefinition,
  input: { pathname: string; search?: URLSearchParams; navigate: ViewContext['navigate']; access: AccessContext },
): ApplicationRouteResult {
  const resolved = resolveAccessibleView(
    application.views ?? [],
    input.pathname,
    input.access,
    application.authentication?.routes?.unauthorized,
  );
  if (!resolved) return { status: 'not-found' };
  if (resolved.status === 'denied') return resolved;

  const context: ViewContext = {
    params: { ...resolved.params },
    search: input.search ?? new URLSearchParams(),
    navigate: input.navigate,
  };
  let element: ReactNode = renderView(resolved.view, context);
  if (resolved.view.layout) {
    const layout = application.layouts?.find((item) => item.id === resolved.view.layout);
    if (!layout) throw new Error(`View "${resolved.view.id}" refers to missing layout "${resolved.view.layout}".`);
    const contents = { ...(resolved.view.layoutContents ?? {}) };
    const slot = layout.slots.find((item) => (item.id === 'content' || item.id === 'main') && !contents[item.id])
      ?? layout.slots.find((item) => !contents[item.id] && (!item.accepts?.length || item.accepts.some((kind) => kind === 'view' || kind === 'component')));
    if (slot) contents[slot.id] = { kind: 'view', content: element };
    element = renderLayout(layout, contents);
  }
  if (application.shell) element = renderShell(application.shell, element);
  return { status: 'rendered', view: resolved.view, params: resolved.params, element };
}

function decodeSegment(value: string) {
  try { return decodeURIComponent(value); } catch { return value; }
}
