import {
  offlineCollection,
  offlineJson,
  retryOfflineSync,
  startOffline,
  stopOffline,
  subscribeOfflineData,
} from './offline';
import { OfflineStatus } from './components/app/offline-status';
import { useEffect, useRef, useState } from 'react';
import { Plus } from 'lucide-react';
import type { OfflineRecord as RecordModel } from './offline';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { useTheme } from 'next-themes';
import type { SidebarTab } from './components/sidebar';
import { taskboardAuth, taskboardShell } from './application';
import { renderView, resolveView, runAction, useAuthentication } from './lib/slopstack';
import { TaskboardViewsProvider } from './views/taskboard-views';
import { createTaskAction, taskboardApplication } from './application';
import { pb } from './api/pocketbase';
import { accentColors, entries, fontSizes, getActiveContainers, lists, type Entry } from './data';
import { SearchTab } from './components/tabs/search-tab';
import { AuthScreen } from './components/app/auth-screen';
import { CreateComposer } from './components/app/create-composer';
import { NotificationsPopover } from './components/app/notifications-popover';
import { applyCustomTheme, readCustomTheme } from './theme';

function compactLocalParts(value: unknown) {
  const match = String(value ?? '').match(/^(\\d{4})(\\d{2})(\\d{2})(?:T(\\d{2})(\\d{2}))?/);
  if (!match) return null;
  return {
    date: `${match[1]}-${match[2]}-${match[3]}`,
    time: match[4] && match[5] ? `${match[4]}:${match[5]}` : undefined,
  };
}

function entryDateTime(record: RecordModel, type: 'task' | 'event') {
  const dateField = type === 'task' ? 'due_date' : 'start_date';
  const localField = type === 'task' ? 'due_local' : 'start_local';
  const allDay = type === 'event' && Boolean(record.all_day);
  const local = compactLocalParts(record[localField]);
  if (allDay || record.time_mode === 'date') {
    return { date: local?.date ?? String(record[dateField] ?? '').slice(0, 10), time: undefined };
  }
  if (record.time_mode === 'floating' && local) return local;
  const parsed = new Date(String(record[dateField] ?? ''));
  if (!Number.isNaN(parsed.getTime())) {
    return {
      date: `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, '0')}-${String(parsed.getDate()).padStart(2, '0')}`,
      time: `${String(parsed.getHours()).padStart(2, '0')}:${String(parsed.getMinutes()).padStart(2, '0')}`,
    };
  }
  return local ?? { date: '', time: undefined };
}

function recordHasRecurrence(record: RecordModel) {
  if (String(record.rrule ?? '').trim()) return true;
  try {
    const stored = JSON.parse(String(record.exdate ?? '')) as {
      properties?: Record<string, unknown>;
    };
    return Array.isArray(stored.properties?.RDATE) && stored.properties.RDATE.length > 0;
  } catch {
    return false;
  }
}

export function App() {
  const auth = useAuthentication(taskboardAuth);
  const authenticated = Boolean(auth.session);
  const authUserId = auth.session?.user.id ?? '';
  const ShellComponent = taskboardShell.component;
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const activeView = resolveView(taskboardApplication.views ?? [], pathname)?.view;
  const activeTab = (activeView?.id ?? 'upcoming') as SidebarTab;
  const [composerOpen, setComposerOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [notificationsClosing, setNotificationsClosing] = useState(false);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const [createType, setCreateType] = useState('task');
  const createColor = accentColors[0].value;
  const [listOnlyComposer, setListOnlyComposer] = useState(false);
  const [taskEventOnlyComposer, setTaskEventOnlyComposer] = useState(false);
  const [editingEntry, setEditingEntry] = useState<import('./data').Entry | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [dataError, setDataError] = useState('');
  const [offlineReadyUser, setOfflineReadyUser] = useState('');
  const [, setDataVersion] = useState(0);
  const loadVersion = useRef(0);
  const offlineReady = useRef<Promise<void>>(Promise.resolve());
  const knownUserId = useRef(pb.authStore.record?.id ?? '');
  const { setTheme } = useTheme();
  useEffect(() => {
    const applyStoredTheme = () => {
      const customTheme = readCustomTheme();
      applyCustomTheme(customTheme);
      if (customTheme) setTheme(customTheme.appearance);
    };
    applyStoredTheme();
    window.addEventListener('taskboard-custom-theme-change', applyStoredTheme);
    return () => window.removeEventListener('taskboard-custom-theme-change', applyStoredTheme);
  }, []);
  const closeNotifications = () => {
    setNotificationsClosing(false);
    setNotificationsOpen(false);
  };

  useEffect(() => {
    const tabName =
      pathname === '/auth/register'
        ? 'Sign up'
        : pathname === '/auth/login'
          ? 'Sign in'
          : notificationsOpen
            ? 'Notifications'
            : {
                upcoming: 'Upcoming',
                calendar: 'Calendar',
                lists: 'Lists',
                people: 'People',
                search: 'Search',
                notifications: 'Notifications',
                settings: 'Settings',
              }[activeTab];
    document.title = `${tabName} | Taskboard`;
  }, [activeTab, notificationsOpen, pathname]);
  useEffect(() => {
    if (!notificationsOpen) return;
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (notificationsRef.current && !notificationsRef.current.contains(event.target as Node))
        closeNotifications();
    };
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    return () => document.removeEventListener('pointerdown', closeOnOutsidePointer);
  }, [notificationsOpen]);
  useEffect(() => {
    const isAuthRoute = pathname.startsWith('/auth/');
    if (!authenticated && !isAuthRoute) void navigate({ to: '/auth/login', replace: true });
    if (authenticated && isAuthRoute) void navigate({ to: '/', replace: true });
  }, [authenticated, navigate, pathname]);
  useEffect(() => {
    void taskboardAuth.initialize();
    return () => taskboardAuth.dispose();
  }, []);
  useEffect(() => {
    if (!authenticated || !authUserId) {
      setOfflineReadyUser('');
      void stopOffline(!pb.authStore.record?.id).catch(() =>
        setDataError(
          'Could not clear saved account data. Close other Taskboard tabs and try again.',
        ),
      );
      return;
    }
    const unsubscribe = subscribeOfflineData(() => setRefreshKey((key) => key + 1));
    offlineReady.current = startOffline(authUserId);
    void offlineReady.current
      .then(() => {
        if (pb.authStore.record?.id === authUserId) setOfflineReadyUser(authUserId);
      })
      .catch(() => {
        if (pb.authStore.record?.id === authUserId) setDataError('Could not open saved data.');
      });
    return () => {
      unsubscribe();
    };
  }, [authenticated, authUserId]);
  useEffect(() => {
    const nextUserId = authUserId;
    if (nextUserId === knownUserId.current) return;
    knownUserId.current = nextUserId;
    loadVersion.current += 1;
    window.dispatchEvent(new CustomEvent('taskboard-notification-count', { detail: 0 }));
    entries.splice(0, entries.length);
    lists.splice(0, lists.length);
    setDataVersion((version) => version + 1);
    setDataError('');
    setComposerOpen(false);
    setEditingEntry(null);
    setListOnlyComposer(false);
    setTaskEventOnlyComposer(false);
  }, [authUserId]);
  useEffect(() => {
    const openCreate = (event: Event) => {
      const type = (event as CustomEvent<{ type?: string }>).detail?.type ?? 'task';
      setEditingEntry(null);
      setCreateType(type);
      setListOnlyComposer(false);
      setTaskEventOnlyComposer(false);
      setComposerOpen(true);
    };
    window.addEventListener('taskboard-create', openCreate);
    const openNotifications = () => {
      setNotificationsClosing(false);
      setNotificationsOpen(true);
    };
    window.addEventListener('taskboard-open-notifications', openNotifications);
    return () => {
      window.removeEventListener('taskboard-create', openCreate);
      window.removeEventListener('taskboard-open-notifications', openNotifications);
    };
  }, []);
  useEffect(() => {
    if (!authenticated || !authUserId) return;
    const userId = authUserId;
    const storedAccent = localStorage.getItem(`taskboard-accent-${userId}`);
    if (!readCustomTheme()) {
      document.documentElement.style.setProperty(
        '--primary',
        storedAccent ?? accentColors[0].value,
      );
    }
    const storedSize = localStorage.getItem(`taskboard-font-size-${userId}`);
    const selectedSize = fontSizes.find((option) => option.value === storedSize);
    document.documentElement.style.setProperty(
      '--app-font-size',
      selectedSize?.size ?? fontSizes[1].size,
    );
  }, [authenticated, authUserId]);
  useEffect(() => {
    if (!authenticated || !authUserId) return;
    const userId = authUserId;
    const version = ++loadVersion.current;
    const isCurrent = () => version === loadVersion.current && pb.authStore.record?.id === userId;
    setDataError('');
    const loadEntries = async () => {
      await offlineReady.current;
      if (!isCurrent()) return;
      const [tasks, events, taskLists, calendars] = await Promise.all([
        offlineCollection('todos').getFullList({ sort: 'due_date' }),
        offlineCollection('events').getFullList({ sort: 'start_date' }),
        offlineCollection('lists').getFullList({ sort: 'name' }),
        offlineCollection('calendars').getFullList({ sort: 'name' }),
      ]);
      const containers = [
        ...taskLists.map((record) => ({
          id: record.id,
          name: String(record.name),
          color: String(record.color),
          description: String(record.description ?? ''),
          kind: 'list' as const,
          archived: Boolean(record.archived),
        })),
        ...calendars.map((record) => ({
          id: record.id,
          name: String(record.name),
          color: String(record.color),
          description: String(record.description ?? ''),
          kind: 'calendar' as const,
          archived: Boolean(record.archived),
        })),
      ];
      const containerById = new Map(containers.map((container) => [container.id, container]));

      const toEntry = (record: RecordModel, type: 'task' | 'event'): Entry | null => {
        const containerId = String(type === 'task' ? record.list : record.calendar);
        const container = containerById.get(containerId);
        if (!container) return null;
        const temporal = entryDateTime(record, type);
        return {
          id: record.id,
          title: String(record.title ?? ''),
          description: String(record.description ?? ''),
          date: temporal.date || new Date().toISOString().slice(0, 10),
          time: temporal.time,
          containerId: container.id,
          list: container.name,
          color: container.color,
          type,
          done: Boolean(record.completed || record.status === 'COMPLETED'),
          fields: record as unknown as Record<string, unknown>,
        };
      };
      const today = new Date();
      // Stable bounds let offline reloads reuse the saved occurrence range.
      const rangeStart = new Date(today.getFullYear() - 1, today.getMonth(), 1);
      const rangeEnd = new Date(today.getFullYear() + 2, today.getMonth(), 1);
      const occurrenceParams = new URLSearchParams({
        start: rangeStart.toISOString(),
        end: rangeEnd.toISOString(),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        max: '5000',
        recurringOnly: 'true',
      });
      let occurrences: Array<{
        id: string;
        masterId: string;
        start: string;
        end: string;
        startLocal?: string;
        endLocal?: string;
        timezone?: string;
        timeMode?: string;
        allDay: boolean;
        recurring: boolean;
        recurrenceId?: string;
        title?: string;
        description?: string;
        calendar?: string;
        color?: string;
      }> = [];
      let occurrenceError = false;
      let occurrencesCached = false;
      try {
        const result = await offlineJson<typeof occurrences>(
          `/api/calendar/occurrences?${occurrenceParams}`,
        );
        occurrences = result.data;
        occurrencesCached = result.cached;
      } catch (error) {
        occurrenceError = true;
        console.warn('Could not load recurring events.', error);
      }
      if (!isCurrent()) return;
      const eventById = new Map(events.map((record) => [record.id, record]));
      const recurringEventIds = new Set(
        events.filter(recordHasRecurrence).map((record) => record.id),
      );
      for (const occurrence of occurrences) {
        if (occurrence.recurring) recurringEventIds.add(occurrence.masterId);
      }
      const eventEntries = occurrences
        .map((occurrence) => {
          if (!recurringEventIds.has(occurrence.masterId)) return null;
          const master = eventById.get(occurrence.masterId);
          if (!master) return null;
          const entry = toEntry(master, 'event');
          if (!entry) return null;
          return {
            ...entry,
            title: occurrence.title ?? entry.title,
            description: occurrence.description ?? entry.description,
            containerId: occurrence.calendar ?? entry.containerId,
            list: containerById.get(occurrence.calendar ?? '')?.name ?? entry.list,
            color: occurrence.color ?? entry.color,
            id: occurrence.id,
            masterId: occurrence.masterId,
            date: occurrence.start.slice(0, 10),
            time: occurrence.allDay ? undefined : occurrence.start.slice(11, 16),
            fields: {
              ...entry.fields,
              start_date: occurrence.start,
              start_local: occurrence.startLocal,
              end_date: occurrence.end,
              end_local: occurrence.endLocal,
              all_day: occurrence.allDay,
              timezone: occurrence.timezone,
              time_mode: occurrence.timeMode,
              recurrence_id: occurrence.recurrenceId,
            },
          };
        })
        .filter((entry): entry is NonNullable<typeof entry> => entry !== null);
      let hiddenContainerIds: string[] = [];
      try {
        hiddenContainerIds = JSON.parse(
          localStorage.getItem(`taskboard-hidden-containers-${userId}`) ?? '[]',
        ) as string[];
      } catch {
        hiddenContainerIds = [];
      }
      const oneOffEvents = events
        .filter(
          (record) =>
            !recurringEventIds.has(record.id) && !String(record.recurrence_parent ?? '').trim(),
        )
        .map((record) => toEntry(record, 'event'))
        .filter((entry): entry is NonNullable<typeof entry> => entry !== null);
      const recurringFallbacks = occurrenceError
        ? events
            .filter((record) => recurringEventIds.has(record.id))
            .map((record) => toEntry(record, 'event'))
            .filter((entry): entry is NonNullable<typeof entry> => entry !== null)
        : [];
      const loadedEntries = [
        ...tasks
          .map((record) => toEntry(record, 'task'))
          .filter((entry): entry is NonNullable<typeof entry> => entry !== null),
        ...oneOffEvents,
        ...eventEntries,
        ...recurringFallbacks,
      ];
      if (!isCurrent()) return;
      const activeContainerIds = new Set(
        getActiveContainers(containers).map((container) => container.id),
      );
      entries.splice(
        0,
        entries.length,
        ...loadedEntries.filter(
          (entry) =>
            activeContainerIds.has(entry.containerId) &&
            !hiddenContainerIds.includes(entry.containerId),
        ),
      );
      lists.splice(
        0,
        lists.length,
        ...containers.map((container) => ({
          id: container.id,
          name: String(container.name),
          color: String(container.color || '#3b6ea8'),
          description: String(container.description || ''),
          kind: container.kind,
          archived: container.archived,
          count: loadedEntries.filter((entry) => entry.containerId === container.id).length,
        })),
      );
      setDataError(
        occurrenceError
          ? 'Recurring events are unavailable. Showing their base events instead.'
          : occurrencesCached
            ? 'Showing saved recurring events. Recent series changes appear after sync.'
            : '',
      );
      setDataVersion((current) => current + 1);
    };
    void loadEntries().catch(() => {
      if (isCurrent()) setDataError('Could not load your entries.');
    });
  }, [authenticated, authUserId, refreshKey]);

  const openCreateComposer = () => {
    void runAction(createTaskAction, undefined, {
      navigate: (to) => {
        void navigate({ to });
      },
      authorize: () => authenticated,
      confirm: () => true,
      notify: () => undefined,
    });
  };

  if (!authenticated)
    return <AuthScreen mode={pathname === '/auth/register' ? 'register' : 'login'} />;
  const editEntry = (entry: import('./data').Entry) => {
    setEditingEntry(entry);
    setCreateType(entry.type);
    setListOnlyComposer(false);
    setTaskEventOnlyComposer(true);
    setComposerOpen(true);
  };
  const bindings = {
    onChanged: () => {
      retryOfflineSync();
      setRefreshKey((key) => key + 1);
    },
    onEdit: editEntry,
    onAddList: () => {
      setEditingEntry(null);
      setCreateType('list');
      setListOnlyComposer(true);
      setTaskEventOnlyComposer(false);
      setComposerOpen(true);
    },
    onOpenMonth: (year: number, month: number) => {
      void navigate({
        to: '/calendar',
        search: { month: `${year}-${String(month + 1).padStart(2, '0')}` },
      });
    },
    onOpenDay: () => {
      setEditingEntry(null);
      setCreateType('task');
      setListOnlyComposer(false);
      setTaskEventOnlyComposer(true);
      setComposerOpen(true);
    },
  };
  const content = activeView ? (
    <TaskboardViewsProvider value={bindings}>
      {renderView(activeView, {
        params: {},
        search: new URLSearchParams(window.location.search),
        navigate: (to) => {
          window.history.pushState({}, '', to);
        },
      })}
    </TaskboardViewsProvider>
  ) : null;

  if (offlineReadyUser !== authUserId) {
    return (
      <ShellComponent>
        <main className="main-content">
          <p role="status">{dataError || 'Loading saved data…'}</p>
        </main>
      </ShellComponent>
    );
  }

  return (
    <ShellComponent>
      <main className="main-content">
        <OfflineStatus />
        {dataError && (
          <p className="form-error app-error" role="alert">
            {dataError}
          </p>
        )}
        <div key={activeTab} className="tab-page-transition">
          {content}
        </div>
      </main>
      {(activeTab === 'upcoming' || activeTab === 'calendar') && (
        <button
          type="button"
          className="button button-primary floating-create-button"
          onClick={openCreateComposer}
          aria-label="New entry"
        >
          <Plus size={20} />
        </button>
      )}
      <SearchTab
        open={searchOpen || pathname === '/search'}
        onOpenChange={(open) => {
          setSearchOpen(open);
          if (!open && pathname === '/search') void navigate({ to: '/' });
        }}
        onSelect={(result) => {
          const entry = entries.find((item) => item.id === result.id);
          if (entry) {
            setEditingEntry(entry);
            setCreateType(entry.type);
            setListOnlyComposer(false);
            setTaskEventOnlyComposer(true);
            setComposerOpen(true);
            setSearchOpen(false);
          } else if (result.type === 'contact') {
            void navigate({ to: '/people' });
            setSearchOpen(false);
          } else if (result.type === 'list' || result.type === 'calendar') {
            void navigate(
              result.type === 'calendar'
                ? { to: '/calendar', search: { month: undefined } }
                : { to: '/lists' },
            );
            setSearchOpen(false);
          }
        }}
        className="search-dialog"
      />
      <div ref={notificationsRef}>
        <NotificationsPopover
          open={notificationsOpen}
          closing={notificationsClosing}
          onClose={closeNotifications}
          onCountChange={(count) =>
            window.dispatchEvent(new CustomEvent('taskboard-notification-count', { detail: count }))
          }
          onAnimationEnd={(event) => {
            if (event.target === event.currentTarget && notificationsClosing) {
              setNotificationsOpen(false);
              setNotificationsClosing(false);
            }
          }}
        />
      </div>
      <CreateComposer
        open={composerOpen}
        listOnly={listOnlyComposer}
        taskEventOnly={taskEventOnlyComposer}
        type={createType}
        color={createColor}
        editing={editingEntry}
        onTypeChange={setCreateType}
        onClose={() => {
          setComposerOpen(false);
          setEditingEntry(null);
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(false);
        }}
        onCreated={() => {
          setEditingEntry(null);
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(false);
          setRefreshKey((key) => key + 1);
        }}
      />
    </ShellComponent>
  );
}
