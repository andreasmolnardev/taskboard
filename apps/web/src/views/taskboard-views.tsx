import { createContext, useContext, type ReactNode } from 'react';
import { CalendarTab } from '../components/tabs/calendar-tab';
import { ListsTab } from '../components/tabs/lists-tab';
import { PeopleTab } from '../components/tabs/people-tab';
import { SettingsTab } from '../components/tabs/settings-tab';
import { UpcomingTab } from '../components/tabs/upcoming-tab';
import type { Entry } from '../data';
import type { ViewContext } from '../lib/slopstack/types';

export type TaskboardViewBindings = {
  onChanged: () => void;
  onEdit: (entry: Entry) => void;
  onAddList: () => void;
  onOpenMonth: (year: number, month: number) => void;
  onOpenDay: () => void;
};

const Bindings = createContext<TaskboardViewBindings | null>(null);

export function TaskboardViewsProvider({
  value,
  children,
}: {
  value: TaskboardViewBindings;
  children: ReactNode;
}) {
  return <Bindings.Provider value={value}>{children}</Bindings.Provider>;
}

function useBindings() {
  const value = useContext(Bindings);
  if (!value) throw new Error('Taskboard views need their route bindings.');
  return value;
}

export function UpcomingView() {
  const { onChanged, onEdit, onOpenMonth } = useBindings();
  return <UpcomingTab onChanged={onChanged} onEdit={onEdit} onOpenMonth={onOpenMonth} />;
}

export function CalendarView({ context }: { context: ViewContext }) {
  const { onEdit, onOpenDay } = useBindings();
  return (
    <CalendarTab
      key={context.search.get('month') ?? 'today'}
      initialMonth={context.search.get('month') ?? undefined}
      onEdit={onEdit}
      onDayClick={onOpenDay}
    />
  );
}

export function ListsView() {
  const { onChanged, onEdit, onAddList } = useBindings();
  return <ListsTab onChanged={onChanged} onEdit={onEdit} onAdd={onAddList} />;
}

export function PeopleView() {
  return <PeopleTab />;
}

export function SearchView() {
  // Search opens as a command dialog over the user's current workspace route.
  return <UpcomingView />;
}

export function SettingsView() {
  const { onChanged } = useBindings();
  return <SettingsTab onChanged={onChanged} />;
}
