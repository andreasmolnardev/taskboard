import { createFileRoutes, type FileRouteModule } from './lib/slopstack';

export const fileRouteModules = import.meta.glob('./routes/**/*.{ts,tsx}', { eager: true }) as Record<
  string,
  FileRouteModule
>;
export const fileRoutes = createFileRoutes(fileRouteModules);

const routeLabels: Record<string, string> = {
  index: 'Upcoming',
  calendar: 'Calendar',
  lists: 'Lists',
  people: 'People',
  search: 'Search',
  settings: 'Settings',
};

export const taskboardViews = fileRoutes.map((route) => ({
  id: route.id === 'index' ? 'upcoming' : route.id,
  path: route.path,
  label: routeLabels[route.id] ?? route.id,
  access: { authenticated: true as const },
  component: route.module.default!,
}));
