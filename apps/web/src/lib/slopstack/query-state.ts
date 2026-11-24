import { useCallback, useEffect, useState } from 'react';
import type { ResourceQuery } from './resource-client';
import type { QueryDefinition } from './types';

const FILTER_PREFIX = 'filter.';

function positiveInteger(value: string | null, fallback: number) {
  if (!value) return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

export function parseResourceQuery(
  search: string | URLSearchParams,
  definition: QueryDefinition = {},
): ResourceQuery {
  const params = typeof search === 'string' ? new URLSearchParams(search.startsWith('?') ? search.slice(1) : search) : search;
  const knownFilters = definition.filters ? new Set(definition.filters.map((filter) => filter.id)) : undefined;
  const filters: Record<string, string> = {};

  for (const [key, value] of params.entries()) {
    if (!key.startsWith(FILTER_PREFIX)) continue;
    const filterId = key.slice(FILTER_PREFIX.length);
    if (knownFilters && !knownFilters.has(filterId)) continue;
    filters[filterId] = value;
  }

  const pageSize = positiveInteger(params.get('size'), definition.pageSize ?? 25);
  return {
    search: params.get('q') || undefined,
    sort: params.get('sort') || definition.defaultSort,
    page: positiveInteger(params.get('page'), 1),
    pageSize,
    filters: Object.keys(filters).length ? filters : undefined,
  };
}

export function writeResourceQuery(
  current: string | URLSearchParams,
  query: ResourceQuery,
) {
  const params = typeof current === 'string'
    ? new URLSearchParams(current.startsWith('?') ? current.slice(1) : current)
    : new URLSearchParams(current);

  for (const key of [...params.keys()]) {
    if (['q', 'sort', 'page', 'size'].includes(key) || key.startsWith(FILTER_PREFIX)) params.delete(key);
  }

  if (query.search?.trim()) params.set('q', query.search.trim());
  if (query.sort) params.set('sort', query.sort);
  if (query.page && query.page > 1) params.set('page', String(query.page));
  if (query.pageSize) params.set('size', String(query.pageSize));
  for (const [filter, value] of Object.entries(query.filters ?? {})) {
    params.set(`${FILTER_PREFIX}${filter}`, Array.isArray(value) ? value.join(',') : String(value));
  }
  return params;
}
/** Keeps a resource query synchronized with the current route's URL search parameters. */
export function useResourceQueryState(definition: QueryDefinition) {
  const read = useCallback(() => parseResourceQuery(window.location.search, definition), [definition]);
  const [query, setQuery] = useState<ResourceQuery>(read);

  useEffect(() => {
    const onPopState = () => setQuery(read());
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [read]);

  const update = useCallback((next: Partial<ResourceQuery>) => {
    setQuery((current) => {
      const updated = { ...current, ...next, page: next.page ?? 1 };
      const params = writeResourceQuery(window.location.search, updated);
      const search = params.toString();
      window.history.replaceState(window.history.state, '', `${window.location.pathname}${search ? `?${search}` : ''}${window.location.hash}`);
      return updated;
    });
  }, []);

  return [query, update] as const;
}
