import { CheckSquare, Home, CalendarDays, List, Users, Search, Settings, Bell } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { defineFieldRenderer } from './lib/slopstack';
import type { FieldRendererProps } from './lib/slopstack';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { createPocketBaseAuthAdapter, createAuthenticationStore } from './lib/slopstack';
import { pb } from './api/pocketbase';
import { ApplicationShell } from './components/patterns/application-shell';
import { taskboardLayout } from './layouts/taskboard-layout';
import {
  defineApplication,
  defineShell,
  defineSettings,
  defineView,
  defineAction,
  createSlopstackRegistry,
} from './lib/slopstack';
import { taskResource, eventResource, listResource, calendarResource } from './resources/taskboard';
import { entries, lists } from './data';
import {
  CalendarView,
  ListsView,
  PeopleView,
  SearchView,
  SettingsView,
  UpcomingView,
} from './views/taskboard-views';
import { NavigationLinks } from './components/patterns/navigation-links';

function TaskboardShell({ children }: { children: ReactNode }) {
  const [notificationCount, setNotificationCount] = useState(0);
  useEffect(() => {
    const updateCount = (event: Event) =>
      setNotificationCount(Number((event as CustomEvent<number>).detail) || 0);
    window.addEventListener('taskboard-notification-count', updateCount);
    return () => window.removeEventListener('taskboard-notification-count', updateCount);
  }, []);
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const activeId =
    pathname === '/calendar'
      ? 'calendar'
      : pathname === '/lists'
        ? 'lists'
        : pathname === '/people'
          ? 'people'
          : pathname === '/settings'
            ? 'settings'
            : pathname === '/search'
              ? 'search'
              : 'upcoming';
  const items = [
    { id: 'upcoming', label: 'Upcoming', icon: Home },
    { id: 'calendar', label: 'Calendar', icon: CalendarDays },
    { id: 'lists', label: 'Lists', icon: List },
    { id: 'people', label: 'People', icon: Users },
    { id: 'search', label: 'Search', icon: Search },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];
  const onNavigate = (id: string) => {
    if (id === 'upcoming') void navigate({ to: '/' });
    else if (id === 'calendar') void navigate({ to: '/calendar', search: { month: undefined } });
    else if (id === 'lists') void navigate({ to: '/lists' });
    else if (id === 'people') void navigate({ to: '/people' });
    else if (id === 'search') void navigate({ to: '/search' });
    else if (id === 'settings') void navigate({ to: '/settings' });
  };
  return (
    <div className="pb-16 md:pb-0" data-taskboard-shell>
      <ApplicationShell
        brand={
          <span className="flex items-center gap-2">
            <CheckSquare className="size-5" />
            Taskboard
          </span>
        }
        items={items}
        activeId={activeId}
        navigationStyle="rail"
        onNavigate={onNavigate}
        account={
          <button
            type="button"
            className="relative rounded-md p-2 text-muted-foreground hover:bg-muted"
            aria-label={`Notifications${notificationCount ? `, ${notificationCount} active` : ''}`}
            onClick={() => window.dispatchEvent(new Event('taskboard-open-notifications'))}
          >
            <Bell className="size-4" />
            {notificationCount > 0 && (
              <span className="absolute -right-1 -top-1 rounded-full bg-destructive px-1.5 text-[10px] leading-4 text-destructive-foreground">
                {notificationCount}
              </span>
            )}
          </button>
        }
      >
        {children}
      </ApplicationShell>
      <nav className="fixed inset-x-0 bottom-0 z-50 border-t bg-background/95 px-2 py-1 backdrop-blur md:hidden">
        <NavigationLinks
          items={items}
          activeId={activeId}
          layout="bottom"
          label="Primary navigation"
          onNavigate={onNavigate}
        />
      </nav>
    </div>
  );
}

const routeViews = [
  ['upcoming', '/', 'Upcoming', UpcomingView],
  ['calendar', '/calendar', 'Calendar', CalendarView],
  ['lists', '/lists', 'Lists', ListsView],
  ['people', '/people', 'People', PeopleView],
  ['search', '/search', 'Search', SearchView],
  ['settings', '/settings', 'Settings', SettingsView],
] as const;
const views = routeViews.map(([id, path, label, component]) =>
  defineView({
    id,
    path,
    label,
    access: { authenticated: true },
    component,
  }),
);

function TaskboardColorField({ field, value, disabled, onChange }: FieldRendererProps) {
  return (
    <label className="grid gap-2 text-sm">
      {field.label}
      <input
        type="color"
        value={String(value || '#2f6f63')}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

export const taskboardColorField = defineFieldRenderer({
  id: 'taskboard-color',
  label: 'Taskboard color picker',
  component: TaskboardColorField,
});

export const taskboardSettings = defineSettings({
  id: 'taskboard-preferences',
  label: 'Taskboard preferences',
  scope: 'user',
  fields: [
    {
      name: 'weekStart',
      label: 'Week starts on',
      kind: 'select',
      options: [
        { label: 'Sunday', value: 'sunday' },
        { label: 'Monday', value: 'monday' },
      ],
    },
    { name: 'accentColor', label: 'Accent color', kind: 'text' },
    {
      name: 'fontSize',
      label: 'Font size',
      kind: 'select',
      options: [
        { label: 'Small', value: 'small' },
        { label: 'Medium', value: 'medium' },
        { label: 'Large', value: 'large' },
      ],
    },
  ],
  defaults: {
    weekStart: 'sunday' as 'sunday' | 'monday',
    accentColor: '#2f6f63' as string,
    fontSize: 'medium' as 'small' | 'medium' | 'large',
  },
});

export const taskboardShell = defineShell({
  id: 'taskboard-shell',
  label: 'Taskboard navigation',
  navigation: [
    { id: 'upcoming', label: 'Upcoming', view: 'upcoming', icon: Home },
    { id: 'calendar', label: 'Calendar', view: 'calendar', icon: CalendarDays },
    { id: 'lists', label: 'Lists', view: 'lists', icon: List },
    { id: 'people', label: 'People', view: 'people', icon: Users },
    { id: 'search', label: 'Search', view: 'search', icon: Search },
    { id: 'settings', label: 'Settings', view: 'settings', icon: Settings },
  ],
  navigationStyles: ['rail', 'bottom'],
  component: TaskboardShell,
});

export const createTaskAction = defineAction({
  id: 'create-task',
  label: 'Create task',
  description: 'Open the Taskboard task composer.',
  keywords: ['new', 'todo'],
  access: { authenticated: true },
  execute: () =>
    window.dispatchEvent(new CustomEvent('taskboard-create', { detail: { type: 'task' } })),
  messages: { success: 'Task composer opened.' },
});

export const taskboardSearchProvider = {
  id: 'taskboard-local',
  label: 'Taskboard local search',
  minQueryLength: 1,
  async search(query: string, signal?: AbortSignal) {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return [];
    if (signal?.aborted) return [];
    const entryResults = entries
      .filter((entry) =>
        `${entry.title} ${entry.description} ${entry.list}`.toLocaleLowerCase().includes(needle),
      )
      .map((entry) => ({
        id: entry.id,
        title: entry.title,
        description: `${entry.type} · ${entry.list}${entry.description ? ` — ${entry.description}` : ''}`,
        keywords: [entry.type, entry.list],
      }));
    const containerResults = lists
      .filter((container) =>
        `${container.name} ${container.description}`.toLocaleLowerCase().includes(needle),
      )
      .map((container) => ({
        id: container.id,
        title: container.name,
        description: `${container.kind} · ${container.description}`,
        keywords: [container.kind],
      }));
    return [...entryResults, ...containerResults];
  },
};

export const taskboardAuth = createAuthenticationStore(createPocketBaseAuthAdapter(pb));

export const taskboardApplication = defineApplication({
  id: 'taskboard',
  name: 'Taskboard',
  backend: { provider: 'pocketbase', capabilities: { database: true, realtime: true } },
  authentication: {
    provider: 'pocketbase',
    enforcement: 'pocketbase-rules',
    routes: { signIn: '/auth/login' },
  },
  shell: taskboardShell,
  resources: [taskResource, eventResource, listResource, calendarResource],
  views,
  layouts: [taskboardLayout],
  settings: [taskboardSettings],
  fieldRenderers: [taskboardColorField],
  searchProviders: [taskboardSearchProvider],
  actions: [createTaskAction],
});

export const taskboardRegistry = createSlopstackRegistry(taskboardApplication).registry;
