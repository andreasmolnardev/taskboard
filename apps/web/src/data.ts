export type Entry = {
  id: string;
  masterId?: string;
  title: string;
  description: string;
  date: string;
  time?: string;
  containerId: string;
  list: string;
  color: string;
  type: 'task' | 'event';
  done?: boolean;
  fields?: Record<string, unknown>;
};

export const accentColors = [
  { name: 'Teal', value: '#2f6f63' },
  { name: 'Blue', value: '#3b6ea8' },
  { name: 'Purple', value: '#7657a8' },
  { name: 'Orange', value: '#c66b32' },
  { name: 'Rose', value: '#b84d68' },
];

export type FontSize = 'small' | 'medium' | 'large';
export const fontSizes: { name: string; value: FontSize; size: string }[] = [
  { name: 'Small', value: 'small', size: '14px' },
  { name: 'Medium', value: 'medium', size: '16px' },
  { name: 'Large', value: 'large', size: '18px' },
];

export const entries: Entry[] = [];

export type EntryContainer = {
  id: string;
  name: string;
  color: string;
  count: number;
  description: string;
  kind: 'list' | 'calendar';
  archived: boolean;
};

export const lists: EntryContainer[] = [];

export function getActiveContainers<T extends Pick<EntryContainer, 'archived'>>(
  containers: readonly T[],
): T[] {
  return containers.filter((container) => !container.archived);
}

export type WeekStart = 'sunday' | 'monday';
export const weekStartStorageKey = 'taskboard-week-start';

export function getWeekStart(): WeekStart {
  return localStorage.getItem(weekStartStorageKey) === 'monday' ? 'monday' : 'sunday';
}

export function formatDateKey(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function formatDate(date: string) {
  return new Intl.DateTimeFormat('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  }).format(new Date(`${date}T12:00:00`));
}
