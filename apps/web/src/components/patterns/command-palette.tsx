import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, Command as CommandIcon, Search } from 'lucide-react';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@/components/ui/command';
import { Button } from '@/components/ui/button';
import { toast } from 'sonner';
import { runAction } from '@/lib/slopstack/action-runtime';
import { filterPaletteActions, filterSearchFallbacks, searchProviders, type PaletteAction } from '@/lib/slopstack/search-runtime';
import type { SearchFallbackDefinition, SearchProvider, SearchResult } from '@/lib/slopstack/types';

export function CommandPalette({
  providers = [],
  navigation = [],
  actions = [],
  searchFallbacks = [],
  onNavigate,
  canAccess = () => true,
  onActionError,
  shortcutLabel = '⌘ K',
  authorize = () => true,
  confirm = (prompt) => window.confirm([prompt.title, prompt.description].filter(Boolean).join('\n\n')),
}: {
  providers?: readonly SearchProvider[];
  navigation?: readonly SearchResult[];
  actions?: readonly PaletteAction[];
  searchFallbacks?: readonly SearchFallbackDefinition[];
  onNavigate: (href: string) => void;
  canAccess?: PaletteActionAccess;
  onActionError?: (error: unknown, action: PaletteAction) => void;
  shortcutLabel?: string;
  authorize?: (requirement?: PaletteAction['access']) => boolean | Promise<boolean>;
  confirm?: (prompt: NonNullable<PaletteAction['confirm']>) => boolean | Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [groups, setGroups] = useState<Awaited<ReturnType<typeof searchProviders>>>([]);
  const [searching, setSearching] = useState(false);
  const visibleActions = useMemo(() => filterPaletteActions(actions.filter((action) => canAccess(action.access)), query), [actions, canAccess, query]);
  const visibleNavigation = useMemo(() => filterNavigation(navigation, query), [navigation, query]);
  const hasResults = Boolean(visibleNavigation.length || visibleActions.length || groups.some((group) => group.results.length));
  const visibleFallbacks = useMemo(() => filterSearchFallbacks(searchFallbacks, query, hasResults || searching, canAccess), [canAccess, hasResults, query, searching, searchFallbacks]);

  useEffect(() => {
    if (!open || !query.trim() || !providers.length) {
      setGroups([]);
      setSearching(false);
      return;
    }
    const controller = new AbortController();
    const timeout = window.setTimeout(() => {
      setSearching(true);
      void searchProviders(providers, query, { signal: controller.signal })
        .then(setGroups)
        .finally(() => { if (!controller.signal.aborted) setSearching(false); });
    }, 120);
    return () => {
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [open, providers, query]);

  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === 'k') {
        event.preventDefault();
        setOpen((value) => !value);
      }
    };
    window.addEventListener('keydown', listener);
    return () => window.removeEventListener('keydown', listener);
  }, []);

  const selectAction = async (action: PaletteAction, input = query) => {
    setOpen(false);
    try {
      await runAction(action, input, {
        navigate: (to) => onNavigate(to),
        authorize,
        confirm,
        notify: ({ title, description, type }) => {
          if (type === 'success') toast.success(title, { description });
          else if (type === 'error') toast.error(title, { description });
          else toast.info(title, { description });
        },
      });
    } catch (error) {
      onActionError?.(error, action);
    }
  };

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setQuery('');
  };

  return (
    <>
      <Button type="button" variant="outline" className="gap-2" onClick={() => setOpen(true)} aria-label="Open command palette">
        <Search className="size-4" />
        <span className="hidden sm:inline">Search or run a command</span>
        <CommandShortcut>{shortcutLabel}</CommandShortcut>
      </Button>
      <CommandDialog open={open} onOpenChange={handleOpenChange} title="Search and commands" description="Search resources, navigate, and run actions.">
        <CommandInput value={query} onValueChange={setQuery} placeholder="Search resources or commands…" />
        <CommandList>
          {!query.trim() && <CommandEmpty>Type to search or choose a command.</CommandEmpty>}
          {visibleNavigation.length > 0 && (
            <CommandGroup heading="Navigation">
              {visibleNavigation.map((result) => <SearchItem key={`nav:${result.id}`} result={result} onSelect={() => result.href && onNavigate(result.href)} />)}
            </CommandGroup>
          )}
          {visibleActions.length > 0 && (
            <CommandGroup heading="Actions">
              {visibleActions.map((action) => (
                <CommandItem key={`action:${action.id}`} value={[action.label, action.description, ...(action.keywords ?? [])].filter(Boolean).join(' ')} onSelect={() => void selectAction(action)}>
                  <CommandIcon className="size-4" />
                  <span>{action.label}</span>
                  {action.description && <span className="ml-2 truncate text-muted-foreground">{action.description}</span>}
                  {action.shortcut && <CommandShortcut>{action.shortcut}</CommandShortcut>}
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          {groups.map((group) => group.results.length > 0 && (
            <CommandGroup key={group.providerId} heading={group.label}>
              {group.results.map((result) => <SearchItem key={`${group.providerId}:${result.id}`} result={result} onSelect={() => result.href && onNavigate(result.href)} />)}
            </CommandGroup>
          ))}
          {visibleFallbacks.length > 0 && (
            <CommandGroup heading="Search fallback">
              {visibleFallbacks.map((fallback) => <CommandItem key={`fallback:${fallback.id}`} value={[fallback.label, fallback.description].filter(Boolean).join(' ')} onSelect={() => void selectAction(fallback, query)}><Search className="size-4" /><span>{fallback.label.replaceAll('{query}', query.trim())}</span>{fallback.description && <span className="ml-2 truncate text-muted-foreground">{fallback.description}</span>}</CommandItem>)}
            </CommandGroup>
          )}
          {searching && <CommandGroup heading="Search"><CommandItem disabled>Searching…</CommandItem></CommandGroup>}
          {query.trim() && !searching && !visibleNavigation.length && !visibleActions.length && !groups.some((group) => group.results.length) && !visibleFallbacks.length && (
            <CommandEmpty>No results found.</CommandEmpty>
          )}
        </CommandList>
      </CommandDialog>
    </>
  );
}

type PaletteActionAccess = (requirement?: PaletteAction['access']) => boolean;

function filterNavigation(items: readonly SearchResult[], query: string) {
  const value = query.trim().toLocaleLowerCase();
  if (!value) return [...items];
  return items.filter((item) => [item.title, item.description, ...(item.keywords ?? [])]
    .filter(Boolean)
    .some((text) => text!.toLocaleLowerCase().includes(value)));
}

function SearchItem({ result, onSelect }: { result: SearchResult; onSelect: () => void }) {
  const Icon = result.icon ?? ArrowRight;
  return (
    <CommandItem value={[result.title, result.description, ...(result.keywords ?? [])].filter(Boolean).join(' ')} onSelect={onSelect}>
      <Icon className="size-4" />
      <span>{result.title}</span>
      {result.description && <span className="ml-2 truncate text-muted-foreground">{result.description}</span>}
    </CommandItem>
  );
}
