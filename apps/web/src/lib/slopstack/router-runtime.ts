import { useSyncExternalStore } from 'react';
import type { AccessContext } from './access-runtime';
import type { ApplicationDefinition, ViewContext } from './types';
import { resolveApplicationRoute, type ApplicationRouteResult } from './view-runtime';

export type NavigationLocation = { pathname: string; search: URLSearchParams };
export type NavigationAdapter = {
  getLocation: () => NavigationLocation;
  navigate: (to: string, options?: { replace?: boolean }) => void;
  back?: () => void;
  subscribe: (listener: () => void) => () => void;
};

export type RouterSnapshot = NavigationLocation & { route: ApplicationRouteResult };

export type SlopstackRouter = {
  getSnapshot: () => RouterSnapshot;
  subscribe: (listener: () => void) => () => void;
  navigate: ViewContext['navigate'];
  back: () => void;
  setAccess: (access: AccessContext) => void;
  dispose: () => void;
};

const BASE_URL = 'http://slopstack.local';

export function createMemoryNavigationAdapter(initial = '/') : NavigationAdapter {
  let history = [new URL(initial, BASE_URL)];
  let index = 0;
  const listeners = new Set<() => void>();
  const emit = () => listeners.forEach((listener) => listener());
  return {
    getLocation: () => ({ pathname: history[index].pathname, search: new URLSearchParams(history[index].search) }),
    navigate(to, options) {
      const target = new URL(to, BASE_URL);
      if (options?.replace) history[index] = target;
      else {
        history = [...history.slice(0, index + 1), target];
        index += 1;
      }
      emit();
    },
    back() {
      if (index <= 0) return;
      index -= 1;
      emit();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

export function createBrowserNavigationAdapter(): NavigationAdapter {
  return {
    getLocation: () => ({ pathname: window.location.pathname, search: new URLSearchParams(window.location.search) }),
    navigate(to, options) {
      if (options?.replace) window.history.replaceState(window.history.state, '', to);
      else window.history.pushState(window.history.state, '', to);
      window.dispatchEvent(new Event('slopstack:navigate'));
    },
    back: () => window.history.back(),
    subscribe(listener) {
      window.addEventListener('popstate', listener);
      window.addEventListener('slopstack:navigate', listener);
      return () => {
        window.removeEventListener('popstate', listener);
        window.removeEventListener('slopstack:navigate', listener);
      };
    },
  };
}

export function createSlopstackRouter(
  application: ApplicationDefinition,
  access: AccessContext,
  adapter: NavigationAdapter = createBrowserNavigationAdapter(),
): SlopstackRouter {
  const listeners = new Set<() => void>();
  let currentAccess = access;
  const navigate: ViewContext['navigate'] = (to, options) => adapter.navigate(to, options);
  const createSnapshot = (): RouterSnapshot => {
    const location = adapter.getLocation();
    return {
      ...location,
      route: resolveApplicationRoute(application, {
        ...location,
        access: currentAccess,
        navigate,
      }),
    };
  };
  let snapshot = createSnapshot();
  const publish = () => {
    snapshot = createSnapshot();
    listeners.forEach((listener) => listener());
  };
  const stop = adapter.subscribe(publish);
  return {
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    navigate,
    back: () => adapter.back?.(),
    setAccess(nextAccess) {
      currentAccess = nextAccess;
      publish();
    },
    dispose() {
      stop();
      listeners.clear();
    },
  };
}

export function useSlopstackRouter(router: SlopstackRouter) {
  return useSyncExternalStore(router.subscribe, router.getSnapshot, router.getSnapshot);
}
