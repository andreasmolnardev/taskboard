import { useEffect, useRef, useState } from 'react';
import { Plus } from 'lucide-react';
import type { RecordModel } from 'pocketbase';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { useTheme } from 'next-themes';
import { Sidebar, type SidebarTab } from './components/sidebar';
import { apiFetch } from './api/client';
import { pb } from './api/pocketbase';
import { accentColors, entries, fontSizes, getActiveContainers, lists, type Entry } from './data';
import { CalendarTab } from './components/tabs/calendar-tab';
import { ListsTab } from './components/tabs/lists-tab';
import { SearchTab } from './components/tabs/search-tab';
import { SettingsTab } from './components/tabs/settings-tab';
import { UpcomingTab } from './components/tabs/upcoming-tab';
import { PeopleTab } from './components/tabs/people-tab';
import { AuthScreen } from './components/app/auth-screen';
import { CreateComposer } from './components/app/create-composer';
import { NotificationsPopover } from './components/app/notifications-popover';

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
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const [authUserId, setAuthUserId] = useState(pb.authStore.record?.id ?? '');
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const calendarSearch = useRouterState({ select: (state) => state.location.search });
  const requestedCalendarMonth =
    typeof calendarSearch.month === 'string' ? calendarSearch.month : undefined;
  const activeTab: SidebarTab =
    pathname === '/calendar'
      ? 'calendar'
      : pathname === '/lists'
        ? 'lists'
        : pathname === '/people'
          ? 'people'
          : pathname === '/search'
            ? 'search'
            : pathname === '/settings'
              ? 'settings'
              : 'upcoming';
  const [composerOpen, setComposerOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [notificationsClosing, setNotificationsClosing] = useState(false);
  const [notificationCount, setNotificationCount] = useState(0);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const [createType, setCreateType] = useState('task');
  const [createColor, setCreateColor] = useState(accentColors[0].value);
  const [listOnlyComposer, setListOnlyComposer] = useState(false);
  const [taskEventOnlyComposer, setTaskEventOnlyComposer] = useState(false);
  const [editingEntry, setEditingEntry] = useState<import('./data').Entry | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [dataError, setDataError] = useState('');
  const [, setDataVersion] = useState(0);
  const loadVersion = useRef(0);
  const knownUserId = useRef(pb.authStore.record?.id ?? '');
  const { resolvedTheme } = useTheme();
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
    return pb.authStore.onChange((_token, record) => {
      const nextUserId = record?.id ?? '';
      const userChanged = nextUserId !== knownUserId.current;
      knownUserId.current = nextUserId;
      if (userChanged) {
        loadVersion.current += 1;
        entries.splice(0, entries.length);
        lists.splice(0, lists.length);
        setDataVersion((version) => version + 1);
        setNotificationCount(0);
        setDataError('');
        setComposerOpen(false);
        setEditingEntry(null);
        setListOnlyComposer(false);
        setTaskEventOnlyComposer(false);
        setAuthUserId(nextUserId);
      }
      setAuthenticated(Boolean(record));
    });
  }, []);
  useEffect(() => {
    if (!authenticated || !authUserId) return;
    const userId = authUserId;
    const storedAccent = localStorage.getItem(`taskboard-accent-${userId}`);
    document.documentElement.style.setProperty(
      '--primary',
      storedAccent ?? accentColors[0].value,
    );
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
    entries.splice(0, entries.length);
    lists.splice(0, lists.length);
    setDataVersion((current) => current + 1);
    setDataError('');
    const loadEntries = async () => {
      const filter = `owner = "${userId}"`;
      const [tasks, events, taskLists, calendars] = await Promise.all([
        pb.collection('todos').getFullList({ filter, sort: 'due_date' }),
        pb.collection('events').getFullList({ filter, sort: 'start_date' }),
        pb.collection('lists').getFullList({ filter, sort: 'name' }),
        pb.collection('calendars').getFullList({ filter, sort: 'name' }),
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
      const rangeStart = new Date();
      rangeStart.setFullYear(rangeStart.getFullYear() - 1);
      const rangeEnd = new Date();
      rangeEnd.setFullYear(rangeEnd.getFullYear() + 2);
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
      }> = [];
      let occurrenceError = false;
      try {
        const occurrenceResponse = await apiFetch(`/api/calendar/occurrences?${occurrenceParams}`, {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
        });
        if (!occurrenceResponse.ok) throw new Error('Could not load recurring events.');
        occurrences = (await occurrenceResponse.json()) as typeof occurrences;
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
          : '',
      );
      setDataVersion((current) => current + 1);
    };
    void loadEntries().catch(() => {
      if (isCurrent()) setDataError('Could not load your entries.');
    });
  }, [authenticated, authUserId, refreshKey]);

  const openCreateComposer = () => {
    setEditingEntry(null);
    setCreateType('task');
    setListOnlyComposer(false);
    setTaskEventOnlyComposer(false);
    setComposerOpen(true);
  };

  if (!authenticated)
    return <AuthScreen mode={pathname === '/auth/register' ? 'register' : 'login'} />;
  const content =
    activeTab === 'calendar' ? (
      <CalendarTab
        key={requestedCalendarMonth ?? 'today'}
        initialMonth={requestedCalendarMonth}
        onEdit={(entry) => {
          setEditingEntry(entry);
          setCreateType(entry.type);
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(true);
          setComposerOpen(true);
        }}
        onDayClick={() => {
          setEditingEntry(null);
          setCreateType('task');
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(true);
          setComposerOpen(true);
        }}
      />
    ) : activeTab === 'lists' ? (
      <ListsTab
        onChanged={() => setRefreshKey((key) => key + 1)}
        onEdit={(entry) => {
          setEditingEntry(entry);
          setCreateType(entry.type);
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(true);
          setComposerOpen(true);
        }}
        onAdd={() => {
          setEditingEntry(null);
          setCreateType('list');
          setListOnlyComposer(true);
          setTaskEventOnlyComposer(false);
          setComposerOpen(true);
        }}
      />
    ) : activeTab === 'people' ? (
      <PeopleTab />
    ) : activeTab === 'settings' ? (
      <SettingsTab onChanged={() => setRefreshKey((key) => key + 1)} />
    ) : (
      <UpcomingTab
        onChanged={() => setRefreshKey((key) => key + 1)}
        onEdit={(entry) => {
          setEditingEntry(entry);
          setCreateType(entry.type);
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(true);
          setComposerOpen(true);
        }}
        onOpenMonth={(year, month) => {
          void navigate({
            to: '/calendar',
            search: { month: `${year}-${String(month + 1).padStart(2, '0')}` },
          });
        }}
      />
    );

  return (
    <div className={`app-shell ${resolvedTheme === 'dark' ? 'dark' : ''}`}>
      <Sidebar
        activeTab={activeTab}
        onChange={(tab) => {
          if (tab === 'notifications') {
            setNotificationsClosing(false);
            setNotificationsOpen(true);
          } else if (tab === 'upcoming') {
            closeNotifications();
            void navigate({ to: '/' });
          } else if (tab === 'calendar') {
            closeNotifications();
            void navigate({ to: '/calendar', search: { month: undefined } });
          } else if (tab === 'lists') {
            closeNotifications();
            void navigate({ to: '/lists' });
          } else if (tab === 'people') {
            closeNotifications();
            void navigate({ to: '/people' });
          } else if (tab === 'search') {
            closeNotifications();
            setSearchOpen(true);
          } else {
            closeNotifications();
            void navigate({ to: '/settings' });
          }
        }}
        notificationCount={notificationCount}
      />
      <main className="main-content">
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
            void navigate({ to: result.type === 'calendar' ? '/settings' : '/lists' });
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
          onCountChange={setNotificationCount}
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
        onColorChange={setCreateColor}
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
    </div>
  );
}
