import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { Link, useNavigate, useRouterState } from '@tanstack/react-router';
import { useTheme } from 'next-themes';
import {
  Bell,
  CalendarDays,
  Check,
  ChevronDown,
  Clock3,
  Filter,
  Moon,
  MoreHorizontal,
  Plus,
  SlidersHorizontal,
  Sun,
  Tag,
  Upload,
  X,
  Zap,
} from 'lucide-react';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from './components/ui/command';
import { Tabs, TabsContent, TabsList, TabsTrigger } from './components/ui/tabs';
import { Sidebar, type SidebarTab } from './components/sidebar';
import { pb } from './api/pocketbase';

type Entry = {
  id: string;
  title: string;
  description: string;
  date: string;
  time?: string;
  list: string;
  color: string;
  type: 'task' | 'event';
  done?: boolean;
};

let entries: Entry[] = [];
/*
  {
    id: '1',
    title: 'Review Q3 product roadmap',
    description: 'Leave notes for the planning session',
    date: '2026-09-08',
    time: '09:30',
    list: 'Work',
    color: '#ef8354',
    type: 'task',
  },
  {
    id: '2',
    title: 'Design critique',
    description: 'Weekly design team sync',
    date: '2026-09-08',
    time: '14:00 - 15:00',
    list: 'Work',
    color: '#ef8354',
    type: 'event',
  },
  {
    id: '3',
    title: 'Buy groceries',
    description: 'Fruit, oat milk, and coffee',
    date: '2026-09-09',
    list: 'Personal',
    color: '#6c9a8b',
    type: 'task',
  },
  {
    id: '4',
    title: 'Dentist appointment',
    description: 'Bring insurance card',
    date: '2026-09-11',
    time: '11:30',
    list: 'Personal',
    color: '#6c9a8b',
    type: 'event',
  },
  {
    id: '5',
    title: 'Send project update',
    description: 'Share the latest milestones with the team',
    date: '2026-09-12',
    list: 'Work',
    color: '#ef8354',
    type: 'task',
  },
  {
    id: '6',
    title: 'Weekend hike',
    description: 'Meet at the north trailhead',
    date: '2026-09-19',
    time: '08:00 - 12:00',
    list: 'Personal',
    color: '#6c9a8b',
    type: 'event',
  },
  {
    id: '7',
    title: 'Pay rent',
    description: 'Monthly payment',
    date: '2026-10-01',
    list: 'Finance',
    color: '#8e7dbe',
    type: 'task',
  },
];

*/
const lists: { name: string; color: string; count: number; description: string }[] = [];

function formatDate(date: string) {
  return new Intl.DateTimeFormat('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  }).format(new Date(`${date}T12:00:00`));
}

function CalendarMonth({ year, month }: { year: number; month: number }) {
  const firstDay = new Date(year, month, 1).getDay();
  const days = new Date(year, month + 1, 0).getDate();
  const monthEntries = entries.filter((entry) => {
    const date = new Date(`${entry.date}T12:00:00`);
    return date.getFullYear() === year && date.getMonth() === month;
  });
  const markedDays = new Set(
    monthEntries.map((entry) => new Date(`${entry.date}T12:00:00`).getDate()),
  );
  const today = new Date();
  const isCurrentMonth = today.getFullYear() === year && today.getMonth() === month;
  return (
    <div className="month-card">
      <h3>
        {new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric' }).format(
          new Date(year, month, 1),
        )}
      </h3>
      <div className="weekdays">
        {['S', 'M', 'T', 'W', 'T', 'F', 'S'].map((day, i) => (
          <span key={`${day}-${i}`}>{day}</span>
        ))}
      </div>
      <div className="calendar-grid">
        {Array.from({ length: firstDay }, (_, index) => (
          <span className="calendar-empty" key={`empty-${index}`} />
        ))}
        {Array.from({ length: days }, (_, index) => {
          const day = index + 1;
          const isToday = isCurrentMonth && day === today.getDate();
          return (
            <span className={`calendar-day ${isToday ? 'today' : ''}`} key={day}>
              <span>{day}</span>
              {markedDays.has(day) && <i />}
            </span>
          );
        })}
      </div>
    </div>
  );
}

function UpcomingView({ onAdd }: { onAdd: () => void }) {
  const now = new Date();
  const [filterOpen, setFilterOpen] = useState(false);
  const [filter, setFilter] = useState<'all' | 'task' | 'event'>('all');
  const visibleEntries = entries.filter((entry) => filter === 'all' || entry.type === filter);
  const grouped = visibleEntries.reduce<Record<string, Entry[]>>((result, entry) => {
    (result[entry.date] ??= []).push(entry);
    return result;
  }, {});
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Upcoming</h1>
        </div>
        <div className="header-actions">
          <div className="filter-wrap">
            <button
              className={`button button-quiet ${filter !== 'all' ? 'selected' : ''}`}
              onClick={() => setFilterOpen(!filterOpen)}
            >
              <Filter size={16} /> Filter <ChevronDown size={15} />
            </button>
            {filterOpen && (
              <div className="popover filter-popover">
                <strong>Show only</strong>
                {(['all', 'task', 'event'] as const).map((value) => (
                  <button
                    key={value}
                    onClick={() => {
                      setFilter(value);
                      setFilterOpen(false);
                    }}
                  >
                    <span className={`radio ${filter === value ? 'checked' : ''}`} />
                    {value === 'all' ? 'Everything' : `${value[0].toUpperCase()}${value.slice(1)}s`}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button className="button button-primary" onClick={onAdd}>
            <Plus size={17} /> New entry
          </button>
        </div>
      </header>
      <section className="calendar-strip" aria-label="Upcoming months">
        <CalendarMonth year={now.getFullYear()} month={now.getMonth()} />
        <CalendarMonth year={now.getFullYear()} month={now.getMonth() + 1} />
      </section>
      <div className="list-legend">
        {lists.map((list) => (
          <span key={list.name}>
            <i style={{ background: list.color }} />
            {list.name}
          </span>
        ))}
      </div>
      <section className="entry-groups">
        {Object.entries(grouped).map(([date, dayEntries]) => (
          <div className="entry-group" key={date}>
            <h2>{formatDate(date)}</h2>
            {dayEntries.map((entry) => (
              <EntryRow entry={entry} key={entry.id} />
            ))}
          </div>
        ))}
      </section>
    </>
  );
}

function EntryRow({ entry }: { entry: Entry }) {
  return (
    <article className="entry-row">
      <div className="entry-time">{entry.time ?? 'All day'}</div>
      <div className="entry-icon" style={{ color: entry.color }}>
        {entry.type === 'event' ? <CalendarDays size={18} /> : <Check size={18} />}
      </div>
      <div className="entry-copy">
        <strong>{entry.title}</strong>
        <span>{entry.description}</span>
      </div>
      <span className="entry-list">
        <i style={{ background: entry.color }} />
        {entry.list}
      </span>
      <button className="icon-button row-more" aria-label={`More options for ${entry.title}`}>
        <MoreHorizontal size={18} />
      </button>
    </article>
  );
}

function ListsView({ onAdd }: { onAdd: () => void }) {
  const [selected, setSelected] = useState('Work');
  const current = lists.find((list) => list.name === selected) ?? {
    name: 'No lists yet',
    color: 'var(--muted)',
    count: 0,
    description: 'Create a list to organize your entries',
  };
  const listEntries = entries.filter((entry) => entry.list === selected);
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Lists</h1>
        </div>
        <button className="button button-primary" onClick={onAdd}>
          <Plus size={17} /> New entry
        </button>
      </header>
      <div className="lists-layout">
        <div className="list-picker">
          {lists.map((list) => (
            <button
              className={`list-picker-item ${selected === list.name ? 'active' : ''}`}
              onClick={() => setSelected(list.name)}
              key={list.name}
            >
              <i style={{ background: list.color }} />
              <span>
                <strong>{list.name}</strong>
                <small>{list.description}</small>
              </span>
              <em>{list.count}</em>
            </button>
          ))}
          <button className="add-list">
            <Plus size={15} /> Add list
          </button>
        </div>
        <section className="selected-list">
          <div className="selected-list-header">
            <div>
              <span className="list-heading">
                <i style={{ background: current.color }} />
                {current.name}
              </span>
              <p>{current.description}</p>
            </div>
            <button className="icon-button">
              <MoreHorizontal size={19} />
            </button>
          </div>
          {listEntries.map((entry) => (
            <EntryRow entry={entry} key={entry.id} />
          ))}
        </section>
      </div>
    </>
  );
}

function SearchView() {
  const [query, setQuery] = useState('');
  const [deep, setDeep] = useState(false);
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Search</h1>
        </div>
      </header>
      <section className="search-panel">
        <div className="search-options">
          <button className={deep ? '' : 'active'} onClick={() => setDeep(false)}>
            <Zap size={15} /> Local
          </button>
          <button className={deep ? 'active' : ''} onClick={() => setDeep(true)}>
            <SlidersHorizontal size={15} /> Deep search
          </button>
          <span>{deep ? 'Searches all synced data' : 'Uses your local cache'}</span>
        </div>
        <Command className="search-command">
          <CommandInput
            autoFocus
            value={query}
            onValueChange={setQuery}
            placeholder="Search your entries"
          />
          <CommandList>
            <CommandEmpty>
              {deep ? 'No entries found in deep search.' : 'No local entries found.'}
            </CommandEmpty>
            <CommandGroup heading={deep ? 'Deep search' : 'Local entries'}>
              {entries.map((entry) => (
                <CommandItem
                  key={entry.id}
                  value={`${entry.title} ${entry.description} ${entry.list}`}
                >
                  <span className="command-entry-icon" style={{ color: entry.color }}>
                    {entry.type === 'event' ? <CalendarDays size={16} /> : <Check size={16} />}
                  </span>
                  <span className="command-entry-copy">
                    <strong>{entry.title}</strong>
                    <small>{entry.description || entry.list}</small>
                  </span>
                  <span className="command-entry-list">{entry.list}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </section>
    </>
  );
}

function SettingsView() {
  const { theme, setTheme } = useTheme();
  const [importedCalendars, setImportedCalendars] = useState<string[]>([]);
  const handleImport = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    setImportedCalendars((current) => [...current, ...files.map((file) => file.name)]);
    event.target.value = '';
  };
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Settings</h1>
        </div>
      </header>
      <section className="settings-card">
        <div className="settings-section">
          <div>
            <h2>Appearance</h2>
            <p>Choose how Taskboard looks on your device.</p>
          </div>
          <div className="theme-toggle">
            <button className={theme !== 'dark' ? 'active' : ''} onClick={() => setTheme('light')}>
              <Sun size={16} /> Light
            </button>
            <button className={theme === 'dark' ? 'active' : ''} onClick={() => setTheme('dark')}>
              <Moon size={16} /> Dark
            </button>
          </div>
        </div>
        <div className="settings-section">
          <div>
            <h2>Account</h2>
            <p>alex@example.com</p>
          </div>
          <button className="button button-quiet">Manage account</button>
        </div>
        <div className="settings-section calendar-settings">
          <div>
            <h2>Calendars</h2>
            <p>Import an iCalendar file or connect a calendar through CalDAV.</p>
            {importedCalendars.length > 0 && (
              <div className="imported-calendars">
                {importedCalendars.map((calendar) => (
                  <span key={calendar}>
                    <CalendarDays size={13} />
                    {calendar}
                  </span>
                ))}
              </div>
            )}
          </div>
          <label className="button button-quiet import-button">
            <Upload size={16} /> Import calendar
            <input type="file" accept=".ics,.ical,text/calendar" multiple onChange={handleImport} />
          </label>
        </div>
        <div className="settings-section">
          <div>
            <h2>Calendar sync</h2>
            <p>CalDAV is connected and syncing.</p>
          </div>
          <span className="status-pill">
            <span /> Connected
          </span>
        </div>
      </section>
    </>
  );
}

function CreateFields({
  title,
  setTitle,
  description,
  setDescription,
  placeholder,
}: {
  title: string;
  setTitle: (value: string) => void;
  description: string;
  setDescription: (value: string) => void;
  placeholder: string;
}) {
  return (
    <div className="create-fields">
      <input
        autoFocus
        className="composer-title"
        placeholder={placeholder}
        value={title}
        onChange={(event) => setTitle(event.target.value)}
      />
      <textarea
        className="composer-description"
        placeholder="Add a description (optional)"
        value={description}
        onChange={(event) => setDescription(event.target.value)}
      />
    </div>
  );
}

function AuthScreen({ mode }: { mode: 'login' | 'register' }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [registrationMode, setRegistrationMode] = useState<'disabled' | 'approval' | 'otp'>(
    'approval',
  );
  useEffect(() => {
    if (mode !== 'register') return;
    void fetch('/api/auth/registration-policy')
      .then((response) => response.json())
      .then((policy: { mode: 'disabled' | 'approval' | 'otp' }) => setRegistrationMode(policy.mode))
      .catch(() => setError('Could not load registration settings.'));
  }, [mode]);
  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    setMessage('');
    try {
      if (mode === 'register') {
        if (registrationMode === 'disabled') return;
        await pb.collection('users').create({ email, password, passwordConfirm: password });
        setMessage(
          registrationMode === 'otp'
            ? 'Account created. Read the activation code in the server logs, then ask an administrator to activate your account.'
            : 'Account created. An administrator must approve it before you can sign in.',
        );
        return;
      }
      await pb.collection('users').authWithPassword(email, password);
    } catch {
      setError(
        mode === 'register'
          ? 'Could not create your account. Check your details.'
          : 'Could not sign in. Check your email and password.',
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <main className="auth-screen">
      <form className="auth-card" onSubmit={handleSubmit}>
        <div className="sidebar-brand">
          <Check size={22} />
        </div>
        <h1>{mode === 'register' ? 'Create your account' : 'Welcome back'}</h1>
        {mode === 'login' && <p>Sign in to see your Taskboard.</p>}
        {message && <div className="auth-message">{message}</div>}
        {mode === 'register' && registrationMode === 'disabled' ? null : (
          <label>
            Email
            <input
              type="email"
              required
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </label>
        )}
        {mode === 'register' && registrationMode === 'disabled' ? null : (
          <label>
            Password
            <input
              type="password"
              required
              autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>
        )}
        {error && <div className="auth-error">{error}</div>}
        {mode === 'register' && registrationMode === 'disabled' ? null : (
          <button className="button button-primary auth-submit" disabled={busy}>
            {busy
              ? mode === 'register'
                ? 'Creating account…'
                : 'Signing in…'
              : mode === 'register'
                ? 'Create account'
                : 'Sign in'}
          </button>
        )}
        <p className="auth-switch">
          {mode === 'register' ? 'Already have an account?' : 'Need an account?'}{' '}
          <Link to={mode === 'register' ? '/auth/login' : '/auth/register'}>
            {mode === 'register' ? 'Sign in' : 'Sign up'}
          </Link>
        </p>
      </form>
    </main>
  );
}

export function App() {
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const activeTab: SidebarTab =
    pathname === '/lists'
      ? 'lists'
      : pathname === '/search'
        ? 'search'
        : pathname === '/settings'
          ? 'settings'
          : 'upcoming';
  const [showComposer, setShowComposer] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [formError, setFormError] = useState('');
  const [createType, setCreateType] = useState('task');
  const [dataVersion, setDataVersion] = useState(0);
  const composerRef = useRef<HTMLDivElement>(null);
  const [composerHeight, setComposerHeight] = useState<number>();
  const { theme } = useTheme();
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
                lists: 'Lists',
                search: 'Search',
                notifications: 'Notifications',
                settings: 'Settings',
              }[activeTab];
    document.title = `${tabName} | Taskboard`;
  }, [activeTab, notificationsOpen]);
  useEffect(() => {
    if (!notificationsOpen) return;
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (notificationsRef.current && !notificationsRef.current.contains(event.target as Node)) {
        setNotificationsOpen(false);
      }
    };
    document.addEventListener('mousedown', closeOnOutsideClick);
    return () => document.removeEventListener('mousedown', closeOnOutsideClick);
  }, [notificationsOpen]);
  useEffect(() => {
    const isAuthRoute = pathname.startsWith('/auth/');
    if (!authenticated && !isAuthRoute) void navigate({ to: '/auth/login', replace: true });
    if (authenticated && isAuthRoute) void navigate({ to: '/', replace: true });
  }, [authenticated, navigate, pathname]);
  useEffect(() => pb.authStore.onChange((_token, record) => setAuthenticated(Boolean(record))), []);
  useEffect(() => {
    if (!authenticated || !pb.authStore.record) return;
    const loadEntries = async () => {
      const records = await pb.collection('todos').getFullList({
        filter: `owner = "${pb.authStore.record?.id}"`,
        sort: 'due_date',
      });
      entries = records.map((record) => ({
        id: record.id,
        title: record.title,
        description: '',
        date: record.due_date?.slice(0, 10) || new Date().toISOString().slice(0, 10),
        list: 'Tasks',
        color: '#87c4a8',
        type: 'task' as const,
        done: record.completed,
      }));
      setDataVersion((version) => version + 1);
    };
    void loadEntries().catch(console.error);
  }, [authenticated]);
  useLayoutEffect(() => {
    if (!showComposer || !composerRef.current) return;
    const composer = composerRef.current;
    const updateHeight = () => setComposerHeight(composer.scrollHeight);
    updateHeight();
    const observer = new ResizeObserver(updateHeight);
    observer.observe(composer);
    return () => observer.disconnect();
  }, [showComposer, createType]);
  const notificationCount = 0;
  const createTabIndex = ['task', 'event', 'list', 'calendar'].indexOf(createType);
  const content = useMemo(() => {
    if (activeTab === 'lists') return <ListsView onAdd={() => setShowComposer(true)} />;
    if (activeTab === 'search') return <SearchView />;
    if (activeTab === 'settings') return <SettingsView />;
    return <UpcomingView onAdd={() => setShowComposer(true)} />;
  }, [activeTab, dataVersion]);
  if (!authenticated)
    return <AuthScreen mode={pathname === '/auth/register' ? 'register' : 'login'} />;
  return (
    <div className={`app-shell ${theme === 'dark' ? 'dark' : ''}`}>
      <Sidebar
        activeTab={activeTab}
        onChange={(tab) => {
          if (tab === 'notifications') {
            setNotificationsOpen(true);
          } else if (tab === 'upcoming') {
            setNotificationsOpen(false);
            void navigate({ to: '/' });
          } else if (tab === 'lists') {
            setNotificationsOpen(false);
            void navigate({ to: '/lists' });
          } else if (tab === 'search') {
            setNotificationsOpen(false);
            void navigate({ to: '/search' });
          } else {
            setNotificationsOpen(false);
            void navigate({ to: '/settings' });
          }
        }}
        notificationCount={notificationCount}
      />
      <main className="main-content">
        <div key={activeTab} className="tab-page-transition">
          {content}
        </div>
        <footer>
          <Tag size={14} /> Synced with CalDAV <span className="sync-dot" />
        </footer>
      </main>
      {notificationsOpen && (
        <div ref={notificationsRef} className="notification-popover">
          <div className="notification-popover-header">
            <div>
              <h2>Notifications</h2>
            </div>
            <button
              className="icon-button"
              onClick={() => setNotificationsOpen(false)}
              aria-label="Close notifications"
            >
              <X size={17} />
            </button>
          </div>
          <div className="empty-state notification-empty">
            <Bell size={22} />
            <strong>No notifications</strong>
            <span>Updates will appear here.</span>
          </div>
          <button
            className="button button-quiet notification-footer"
            onClick={() => setNotificationsOpen(false)}
          >
            Mark all as read
          </button>
        </div>
      )}
      {showComposer && (
        <div className="modal-backdrop" onMouseDown={() => setShowComposer(false)}>
          <div
            ref={composerRef}
            className="composer create-modal"
            style={{ height: composerHeight ? `${composerHeight}px` : undefined }}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="composer-header">
              <div>
                <h2>Create new</h2>
                <p className="modal-subtitle">Add something to your Taskboard.</p>
              </div>
              <button
                className="icon-button"
                onClick={() => setShowComposer(false)}
                aria-label="Close"
              >
                <X size={19} />
              </button>
            </div>
            <Tabs value={createType} onValueChange={setCreateType} className="create-tabs">
              <TabsList className="create-tabs-list">
                <span
                  className="create-tabs-track"
                  style={{ transform: `translateX(calc(${createTabIndex} * (100% + 3px)))` }}
                  aria-hidden="true"
                />
                <TabsTrigger value="task">Task</TabsTrigger>
                <TabsTrigger value="event">Event</TabsTrigger>
                <TabsTrigger value="list">List</TabsTrigger>
                <TabsTrigger value="calendar">Calendar</TabsTrigger>
              </TabsList>
              <TabsContent value="task">
                <CreateFields
                  title={title}
                  setTitle={setTitle}
                  description={description}
                  setDescription={setDescription}
                  placeholder="What needs doing?"
                />
              </TabsContent>
              <TabsContent value="event">
                <CreateFields
                  title={title}
                  setTitle={setTitle}
                  description={description}
                  setDescription={setDescription}
                  placeholder="What is happening?"
                />
                <div className="composer-meta">
                  <button>
                    <Clock3 size={16} /> Today
                  </button>
                  <button>
                    <Clock3 size={16} /> Add time
                  </button>
                </div>
              </TabsContent>
              <TabsContent value="list">
                <CreateFields
                  title={title}
                  setTitle={setTitle}
                  description={description}
                  setDescription={setDescription}
                  placeholder="Name this list"
                />
              </TabsContent>
              <TabsContent value="calendar">
                <CreateFields
                  title={title}
                  setTitle={setTitle}
                  description={description}
                  setDescription={setDescription}
                  placeholder="Name this calendar"
                />
              </TabsContent>
            </Tabs>
            {formError && <p className="form-error">{formError}</p>}
            <button
              className="button button-primary composer-save"
              onClick={async () => {
                if (!title.trim()) return;
                if (createType !== 'task') {
                  setFormError(
                    'This type is not available until its PocketBase collection is configured.',
                  );
                  return;
                }
                try {
                  const record = await pb.collection('todos').create({
                    owner: pb.authStore.record?.id,
                    title,
                    completed: false,
                    due_date: new Date().toISOString(),
                  });
                  entries = [
                    ...entries,
                    {
                      id: record.id,
                      title,
                      description: '',
                      date: new Date().toISOString().slice(0, 10),
                      list: 'Tasks',
                      color: '#87c4a8',
                      type: 'task',
                    },
                  ];
                  setDataVersion((version) => version + 1);
                  setTitle('');
                  setDescription('');
                  setFormError('');
                  setShowComposer(false);
                } catch {
                  setFormError('Could not create this task.');
                }
              }}
            >
              <Plus size={17} /> Create {createType}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
