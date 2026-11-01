import { useEffect, useState } from 'react';
import { CalendarDays, Check } from 'lucide-react';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../ui/command';
import { apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';
import { entries, lists } from '../../data';
import { taskboardSearchProvider } from '../../application';

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
  const [deepResults, setDeepResults] = useState<DeepResult[]>([]);
  const [localResults, setLocalResults] = useState<DeepResult[]>([]);

  useEffect(() => {
    if (mode !== 'local') return;
    const queryText = query.trim();
    if (!queryText) {
      setLocalResults([]);
      return;
    }
    const controller = new AbortController();
    void taskboardSearchProvider.search(queryText, controller.signal).then((matches) => {
      if (controller.signal.aborted) return;
      setLocalResults(
        matches.map((match) => {
          const entry = entries.find((item) => item.id === match.id);
          const container = lists.find((item) => item.id === match.id);
          return {
            id: match.id,
            type:
              entry?.type === 'task'
                ? 'todo'
                : entry?.type === 'event'
                  ? 'event'
                  : (container?.kind ?? 'list'),
            title: match.title,
            description: match.description,
            container: entry?.list,
          };
        }),
      );
    });
    return () => controller.abort();
  }, [mode, query]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (mode !== 'deep' || !query.trim()) {
      setDeepResults([]);
      setError('');
      return;
    }
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setLoading(true);
      setError('');
      const params = new URLSearchParams({ query: query.trim(), perPage: '50' });
      void apiFetch(`/api/search?${params}`, {
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
        signal: controller.signal,
      })
        .then(async (response) => {
          if (!response.ok) throw new Error('Deep search failed.');
          return (await response.json()) as { items: DeepResult[] };
        })
        .then((response) => setDeepResults(response.items))
        .catch((reason: unknown) => {
          if (!controller.signal.aborted)
            setError(reason instanceof Error ? reason.message : 'Deep search failed.');
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, 250);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [mode, query]);

  const results = mode === 'local' ? localResults : deepResults;
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
