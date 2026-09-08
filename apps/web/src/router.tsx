import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { App } from './app';

const rootRoute = createRootRoute({ component: App });

const upcomingRoute = createRoute({ getParentRoute: () => rootRoute, path: '/' });
const listsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/lists' });
const searchRoute = createRoute({ getParentRoute: () => rootRoute, path: '/search' });
const settingsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/settings' });
const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: '/auth/login' });
const registerRoute = createRoute({ getParentRoute: () => rootRoute, path: '/auth/register' });

const routeTree = rootRoute.addChildren([
  upcomingRoute,
  listsRoute,
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
