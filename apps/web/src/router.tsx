import { useCallback, useSyncExternalStore } from 'react';

type LocationSnapshot = { location: { pathname: string; search: URLSearchParams } };
type NavigateOptions = { to: string; replace?: boolean; search?: Record<string, string | undefined> };

function subscribe(listener: () => void) {
  window.addEventListener('popstate', listener);
  window.addEventListener('slopstack:navigate', listener);
  return () => {
    window.removeEventListener('popstate', listener);
    window.removeEventListener('slopstack:navigate', listener);
  };
}

export function useRouterState<T>({ select }: { select: (state: LocationSnapshot) => T }): T {
  const href = useSyncExternalStore(
    subscribe,
    () => `${window.location.pathname}${window.location.search}`,
    () => '/',
  );
  const url = new URL(href, 'http://taskboard.local');
  return select({ location: { pathname: url.pathname, search: url.searchParams } });
}

export function useNavigate() {
  return useCallback((target: string | NavigateOptions) => {
    const options = typeof target === 'string' ? { to: target } : target;
    const url = new URL(options.to, window.location.href);
    if (options.search) {
      url.search = '';
      for (const [key, value] of Object.entries(options.search)) {
        if (value !== undefined) url.searchParams.set(key, value);
      }
    }
    const path = `${url.pathname}${url.search}${url.hash}`;
    if (options.replace) window.history.replaceState(window.history.state, '', path);
    else window.history.pushState(window.history.state, '', path);
    window.dispatchEvent(new Event('slopstack:navigate'));
  }, []);
}
