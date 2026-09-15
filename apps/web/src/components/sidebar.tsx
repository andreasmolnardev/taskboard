import { Bell, CalendarDays, CheckSquare, Home, List, Search, Settings, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

type SidebarTab =
  'upcoming' | 'calendar' | 'lists' | 'people' | 'search' | 'notifications' | 'settings';

type SidebarProps = {
  activeTab: SidebarTab;
  onChange: (tab: SidebarTab) => void;
  notificationCount: number;
};

const items: { id: SidebarTab; label: string; icon: LucideIcon }[] = [
  { id: 'upcoming', label: 'Upcoming', icon: Home },
  { id: 'calendar', label: 'Calendar', icon: CalendarDays },
  { id: 'lists', label: 'Lists', icon: List },
  { id: 'people', label: 'People', icon: Users },
  { id: 'search', label: 'Search', icon: Search },
  { id: 'notifications', label: 'Notifications', icon: Bell },
  { id: 'settings', label: 'Settings', icon: Settings },
];

export function Sidebar({ activeTab, onChange, notificationCount }: SidebarProps) {
  return (
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-brand" aria-label="Taskboard">
        <CheckSquare size={21} strokeWidth={2.5} />
      </div>
      <nav className="sidebar-nav">
        {items.map(({ id, label, icon: Icon }) => (
          <button
            className={`sidebar-link ${activeTab === id ? 'is-active' : ''}`}
            key={id}
            onClick={() => onChange(id)}
            aria-label={label}
            aria-current={activeTab === id ? 'page' : undefined}
          >
            <Icon size={20} />
            <span className="sidebar-tooltip">{label}</span>
            {id === 'notifications' && notificationCount > 0 && (
              <span className="notification-badge">{notificationCount}</span>
            )}
          </button>
        ))}
      </nav>
    </aside>
  );
}

export type { SidebarTab };
