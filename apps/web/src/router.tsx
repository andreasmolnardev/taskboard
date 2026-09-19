import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { App } from './app';

const rootRoute = createRootRoute({ component: App });

const upcomingRoute = createRoute({ getParentRoute: () => rootRoute, path: '/' });
const calendarRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/calendar',
  validateSearch: (search: Record<string, unknown>) => ({
    month: typeof search.month === 'string' ? search.month : undefined,
  }),
});
const listsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/lists' });
const peopleRoute = createRoute({ getParentRoute: () => rootRoute, path: '/people' });
const searchRoute = createRoute({ getParentRoute: () => rootRoute, path: '/search' });

const settingsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/settings' });
const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: '/auth/login' });
const registerRoute = createRoute({ getParentRoute: () => rootRoute, path: '/auth/register' });

const routeTree = rootRoute.addChildren([
  upcomingRoute,
  calendarRoute,
  listsRoute,
  peopleRoute,
  searchRoute,

  settingsRoute,
  loginRoute,
  registerRoute,
]);

export const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
