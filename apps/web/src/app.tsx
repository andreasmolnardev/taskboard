import { useEffect, useRef, useState } from 'react';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { useTheme } from 'next-themes';
import { Sidebar, type SidebarTab } from './components/sidebar';
import { pb } from './api/pocketbase';
import { accentColors, entries, fontSizes } from './data';
import { CalendarTab } from './components/tabs/calendar-tab';
import { ListsTab } from './components/tabs/lists-tab';
import { SearchTab } from './components/tabs/search-tab';
import { SettingsTab } from './components/tabs/settings-tab';
import { UpcomingTab } from './components/tabs/upcoming-tab';
import { AuthScreen } from './components/app/auth-screen';
import { CreateComposer } from './components/app/create-composer';
import { NotificationsPopover } from './components/app/notifications-popover';

export function App() {
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const activeTab: SidebarTab =
    pathname === '/calendar'
      ? 'calendar'
      : pathname === '/lists'
        ? 'lists'
        : pathname === '/search'
          ? 'search'
          : pathname === '/settings'
            ? 'settings'
            : 'upcoming';
  const [composerOpen, setComposerOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [notificationsClosing, setNotificationsClosing] = useState(false);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const [createType, setCreateType] = useState('task');
  const [createColor, setCreateColor] = useState(accentColors[0].value);
  const [listOnlyComposer, setListOnlyComposer] = useState(false);
  const [taskEventOnlyComposer, setTaskEventOnlyComposer] = useState(false);
  const [, setDataVersion] = useState(0);
  const { resolvedTheme } = useTheme();
  const closeNotifications = () => setNotificationsClosing(true);

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
                search: 'Search',
                notifications: 'Notifications',
                settings: 'Settings',
              }[activeTab];
    document.title = `${tabName} | Taskboard`;
  }, [activeTab, notificationsOpen, pathname]);
  useEffect(() => {
    if (!notificationsOpen) return;
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (notificationsRef.current && !notificationsRef.current.contains(event.target as Node))
        closeNotifications();
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
    const userId = pb.authStore.record.id;
    const storedSize = localStorage.getItem(`taskboard-font-size-${userId}`);
    const selectedSize = fontSizes.find((option) => option.value === storedSize);
    document.documentElement.style.setProperty('--app-font-size', selectedSize?.size ?? fontSizes[1].size);
  }, [authenticated]);
  useEffect(() => {
    if (!authenticated || !pb.authStore.record) return;
    const loadEntries = async () => {
      const records = await pb
        .collection('todos')
        .getFullList({ filter: `owner = "${pb.authStore.record?.id}"`, sort: 'due_date' });
      entries.splice(
        0,
        entries.length,
        ...records.map((record) => ({
          id: record.id,
          title: record.title,
          description: '',
          date: record.due_date?.slice(0, 10) || new Date().toISOString().slice(0, 10),
          list: 'Tasks',
          color: '#87c4a8',
          type: 'task' as const,
          done: record.completed,
        })),
      );
      setDataVersion((version) => version + 1);
    };
    void loadEntries().catch(console.error);
  }, [authenticated]);

  if (!authenticated)
    return <AuthScreen mode={pathname === '/auth/register' ? 'register' : 'login'} />;
  const content =
    activeTab === 'calendar' ? (
      <CalendarTab
        onDayClick={() => {
          setCreateType('task');
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(true);
          setComposerOpen(true);
        }}
      />
    ) : activeTab === 'lists' ? (
      <ListsTab
        onAdd={() => {
          setCreateType('list');
          setListOnlyComposer(true);
          setTaskEventOnlyComposer(false);
          setComposerOpen(true);
        }}
      />
    ) : activeTab === 'settings' ? (
      <SettingsTab />
    ) : (
      <UpcomingTab
        onAdd={() => {
          setCreateType('task');
          setListOnlyComposer(false);
          setTaskEventOnlyComposer(false);
          setComposerOpen(true);
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
            void navigate({ to: '/calendar' });
          } else if (tab === 'lists') {
            closeNotifications();
            void navigate({ to: '/lists' });
          } else if (tab === 'search') {
            closeNotifications();
            setSearchOpen(true);
          } else {
            closeNotifications();
            void navigate({ to: '/settings' });
          }
        }}
        notificationCount={0}
      />
      <main className="main-content">
        <div key={activeTab} className="tab-page-transition">
          {content}
        </div>
      </main>
      <SearchTab open={searchOpen} onOpenChange={setSearchOpen} className="search-dialog" />
      {notificationsOpen && (
        <div ref={notificationsRef}>
          <NotificationsPopover
            closing={notificationsClosing}
            onClose={closeNotifications}
            onAnimationEnd={(event) => {
              if (event.target === event.currentTarget && notificationsClosing) {
                setNotificationsOpen(false);
                setNotificationsClosing(false);
              }
            }}
          />
        </div>
      )}
      <CreateComposer
        open={composerOpen}
        listOnly={listOnlyComposer}
        taskEventOnly={taskEventOnlyComposer}
        type={createType}
        color={createColor}
        onTypeChange={setCreateType}
        onColorChange={setCreateColor}
        onClose={() => setComposerOpen(false)}
        onCreated={() => setDataVersion((version) => version + 1)}
      />
    </div>
  );
}
