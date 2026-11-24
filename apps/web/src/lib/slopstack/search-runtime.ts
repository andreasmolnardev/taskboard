import type { ActionDefinition, PermissionRequirement, SearchFallbackDefinition, SearchProvider, SearchResult, ShortcutDefinition } from './types';

export type SearchResultGroup = {
  providerId: string;
  label: string;
  results: readonly SearchResult[];
  error?: string;
};

export async function searchProviders(
  providers: readonly SearchProvider[],
  query: string,
  options: { signal?: AbortSignal; limitPerProvider?: number; onProviderError?: (provider: SearchProvider, error: unknown) => void } = {},
): Promise<readonly SearchResultGroup[]> {
  const normalizedQuery = query.trim();
  if (!normalizedQuery || options.signal?.aborted) return [];

  const eligible = providers.filter((provider) => normalizedQuery.length >= (provider.minQueryLength ?? 1));
  const groups: Array<SearchResultGroup | undefined> = await Promise.all(eligible.map(async (provider): Promise<SearchResultGroup | undefined> => {
    try {
      const results = await provider.search(normalizedQuery, options.signal);
      if (options.signal?.aborted) return undefined;
      const unique = new Map<string, SearchResult>();
      for (const result of results) {
        const key = result.href ? `href:${result.href}` : `id:${result.id}`;
        if (!unique.has(key)) unique.set(key, result);
        if (unique.size >= (options.limitPerProvider ?? 8)) break;
      }
      return { providerId: provider.id, label: provider.label, results: [...unique.values()] };
    } catch (error) {
      if (options.signal?.aborted) return undefined;
      options.onProviderError?.(provider, error);
      return { providerId: provider.id, label: provider.label, results: [], error: error instanceof Error ? error.message : 'Search failed.' };
    }
  }));

  return groups.filter((group): group is SearchResultGroup => Boolean(group));
}

export type PaletteAction = ActionDefinition<string, unknown>;
export type PaletteActionDescriptor = Pick<PaletteAction, 'id' | 'label' | 'description' | 'keywords' | 'shortcut' | 'access'>;

export function filterPaletteActions<Action extends PaletteActionDescriptor>(actions: readonly Action[], query: string): Action[] {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (!normalizedQuery) return [...actions];
  return actions.filter((action) => [action.label, action.description, ...(action.keywords ?? [])]
    .filter(Boolean)
    .some((value) => value!.toLocaleLowerCase().includes(normalizedQuery)));
}

export function filterSearchFallbacks(
  fallbacks: readonly SearchFallbackDefinition[],
  query: string,
  hasResults: boolean,
  canAccess: (requirement?: PermissionRequirement) => boolean = () => true,
) {
  const normalized = query.trim();
  return fallbacks.filter((fallback) => normalized.length >= (fallback.minQueryLength ?? 1)
    && (!fallback.matches || fallback.matches(normalized))
    && (!(fallback.whenNoResults ?? true) || !hasResults)
    && canAccess(fallback.access));
}

export function defineSearchAction(options: {
  id: string;
  label: string;
  viewPath: string;
  queryParameter?: string;
  description?: string;
  keywords?: readonly string[];
  access?: PermissionRequirement;
}): ActionDefinition<string> {
  return {
    id: options.id,
    label: options.label,
    description: options.description,
    keywords: options.keywords,
    access: options.access,
    execute({ input, navigate }) {
      const target = new URL(options.viewPath, 'http://slopstack.local');
      target.searchParams.set(options.queryParameter ?? 'q', input.trim());
      navigate(target.pathname + target.search + target.hash);
    },
  };
}

export function normalizeShortcutKeys(keys: readonly string[]) {
  return keys.map((key) => {
    const value = key.toLocaleLowerCase();
    if (['cmd', 'command', 'meta', 'ctrlorcmd', 'mod'].includes(value)) return 'mod';
    if (value === 'control') return 'ctrl';
    if (value === 'option') return 'alt';
    return value;
  }).sort((a, b) => {
    const order = ['mod', 'ctrl', 'alt', 'shift'];
    const rank = (key: string) => {
      const index = order.indexOf(key);
      return index < 0 ? order.length : index;
    };
    return rank(a) - rank(b);
  });
}

function shortcutSignature(shortcut: ShortcutDefinition) {
  return normalizeShortcutKeys(shortcut.keys).join('+');
}

export class ShortcutRegistry {
  private readonly byId = new Map<string, ShortcutDefinition>();
  private readonly byKeys = new Map<string, ShortcutDefinition>();

  constructor(shortcuts: readonly ShortcutDefinition[] = []) {
    shortcuts.forEach((shortcut) => this.register(shortcut));
  }

  register(shortcut: ShortcutDefinition) {
    if (this.byId.has(shortcut.id)) throw new Error(`Duplicate shortcut id "${shortcut.id}".`);
    const signature = shortcutSignature(shortcut);
    if (this.byKeys.has(signature)) throw new Error(`Shortcut "${shortcut.id}" conflicts with "${this.byKeys.get(signature)?.id}" on ${signature}.`);
    this.byId.set(shortcut.id, shortcut);
    this.byKeys.set(signature, shortcut);
    return () => {
      this.byId.delete(shortcut.id);
      this.byKeys.delete(signature);
    };
  }

  getAll() {
    return [...this.byId.values()];
  }

  find(event: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>) {
    for (const shortcut of this.byId.values()) {
      if (matchesShortcut(event, shortcut)) return shortcut;
    }
    return undefined;
  }
}

export function matchesShortcut(
  event: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>,
  shortcut: ShortcutDefinition,
) {
  const keys = normalizeShortcutKeys(shortcut.keys);
  const primary = keys.filter((key) => !['mod', 'ctrl', 'alt', 'shift'].includes(key));
  if (primary.length !== 1 || primary[0] !== event.key.toLocaleLowerCase()) return false;
  const wantsMod = keys.includes('mod');
  const wantsCtrl = keys.includes('ctrl');
  return (wantsMod ? event.metaKey || event.ctrlKey : event.metaKey === false)
    && (wantsCtrl ? event.ctrlKey : wantsMod || !event.ctrlKey)
    && event.altKey === keys.includes('alt')
    && event.shiftKey === keys.includes('shift');
}

export function installShortcutRegistry(
  registry: ShortcutRegistry,
  options: {
    authorize?: (access?: PermissionRequirement) => boolean | Promise<boolean>;
    onError?: (error: unknown, shortcut: ShortcutDefinition) => void;
    target?: Pick<Window, 'addEventListener' | 'removeEventListener'>;
  } = {},
) {
  const target = options.target ?? window;
  const listener = (event: Event) => {
    const keyboardEvent = event as KeyboardEvent;
    const shortcut = registry.find(keyboardEvent);
    if (!shortcut) return;
    const element = keyboardEvent.target;
    const isEditing = element instanceof HTMLElement && (
      element.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(element.tagName)
    );
    if (isEditing && !keyboardEvent.metaKey && !keyboardEvent.ctrlKey) return;

    void (async () => {
      if (options.authorize && !(await options.authorize(shortcut.access))) return;
      keyboardEvent.preventDefault();
      try {
        await shortcut.execute();
      } catch (error) {
        options.onError?.(error, shortcut);
      }
    })();
  };
  target.addEventListener('keydown', listener);
  return () => target.removeEventListener('keydown', listener);
}
