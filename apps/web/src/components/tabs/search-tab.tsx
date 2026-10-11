import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CalendarDays, Check } from 'lucide-react';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../ui/command';
import { apiJson } from '../../api/client';
import { pb } from '../../api/pocketbase';
import { taskboardQueryKeys } from '../../api/query-keys';
import { entries } from '../../data';

type DeepResult = {
  id: string;
  type: 'todo' | 'event' | 'list' | 'calendar' | 'contact';
  title: string;
  description?: string;
  container?: string;
};

export function SearchTab({
  open,
  onOpenChange,
  className,
  onSelect,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect?: (result: DeepResult) => void;
  className?: string;
}) {
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<'local' | 'deep'>('local');
  const [debouncedQuery, setDebouncedQuery] = useState('');

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 250);
    return () => window.clearTimeout(timer);
  }, [query]);

  const userId = pb.authStore.record?.id ?? '';
  const params = new URLSearchParams({ query: debouncedQuery, perPage: '50' });
  const deepSearch = useQuery({
    queryKey: taskboardQueryKeys.search(userId, debouncedQuery),
    enabled: open && mode === 'deep' && Boolean(userId && debouncedQuery),
    queryFn: ({ signal }) =>
      apiJson<{ items: DeepResult[] }>(`/api/search?${params}`, { signal }),
  });
  const loading = mode === 'deep' && deepSearch.isFetching;
  const error = mode === 'deep' && deepSearch.error instanceof Error ? deepSearch.error.message : '';

  const results =
    mode === 'local'
      ? entries.map((entry) => ({
          id: entry.id,
          type: entry.type === 'task' ? ('todo' as const) : ('event' as const),
          title: entry.title,
          description: entry.description,
          container: entry.list,
        }))
      : deepSearch.data?.items ?? [];
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      className={className}
      title="Search Taskboard"
      description="Search your tasks and events"
    >
      <CommandInput
        autoFocus
        value={query}
        onValueChange={setQuery}
        placeholder="Search your entries"
      />
      <div className="theme-toggle" role="group" aria-label="Search mode">
        <button className={mode === 'local' ? 'active' : ''} onClick={() => setMode('local')}>
          Local
        </button>
        <button className={mode === 'deep' ? 'active' : ''} onClick={() => setMode('deep')}>
          Deep
        </button>
      </div>
      <CommandList>
        <CommandEmpty>{loading ? 'Searching…' : error || 'No entries found.'}</CommandEmpty>
        <CommandGroup heading={mode === 'local' ? 'Loaded entries' : 'All results'}>
          {results.map((entry) => (
            <CommandItem
              key={`${entry.type}-${entry.id}`}
              value={`${entry.title} ${entry.description ?? ''} ${entry.container ?? ''}`}
              onSelect={() => onSelect?.(entry)}
            >
              <span className="command-entry-icon">
                {entry.type === 'event' ? <CalendarDays size={16} /> : <Check size={16} />}
              </span>
              <span className="command-entry-copy">
                <strong>{entry.title}</strong>
                <small>{entry.description || entry.type}</small>
              </span>
              <span className="command-entry-list">{entry.container}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
