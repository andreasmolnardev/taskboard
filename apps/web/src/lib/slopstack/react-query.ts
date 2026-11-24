import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type PocketBase from 'pocketbase';
import type { ResourceClient, ResourceQuery } from './resource-client';
import type { ResourceDefinition } from './types';

const resourceQueryKey = (resourceId: string) => ['slopstack', 'resources', resourceId] as const;

export function useResourceQuery<RecordType extends object, CreateType extends object, UpdateType extends object>(
  client: ResourceClient<RecordType, CreateType, UpdateType>,
  query: ResourceQuery = {},
) {
  const keyQuery = Object.fromEntries(Object.entries(query).filter(([key]) => key !== 'signal'));
  const result = useQuery({
    queryKey: [...resourceQueryKey(client.definition.id), keyQuery],
    queryFn: ({ signal }) => client.list({ ...query, signal }),
  });
  return { ...result, rows: result.data?.items ?? [], total: result.data?.total ?? 0 };
}

export function useResourceMutations<RecordType extends object, CreateType extends object, UpdateType extends object>(
  client: ResourceClient<RecordType, CreateType, UpdateType>,
) {
  const queryClient = useQueryClient();
  const queryKey = resourceQueryKey(client.definition.id);

  const create = useMutation({
    mutationFn: client.create,
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });

  const update = useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateType }) => client.update(id, input),
    onMutate: async ({ id, input }) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueriesData({ queryKey });
      const identify = client.definition.identify as ((record: RecordType) => string) | undefined;
      if (identify) {
        queryClient.setQueriesData({ queryKey }, (data: unknown) => {
          if (!data || typeof data !== 'object' || !('items' in data) || !Array.isArray(data.items)) return data;
          return {
            ...data,
            items: data.items.map((record: RecordType) => identify(record) === id ? { ...record, ...input } : record),
          };
        });
      }
      return { previous };
    },
    onError: (_error, _variables, context) => {
      context?.previous.forEach(([key, data]) => queryClient.setQueryData(key, data));
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });

  const remove = useMutation({
    mutationFn: client.delete,
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueriesData({ queryKey });
      const identify = client.definition.identify as ((record: RecordType) => string) | undefined;
      if (identify) {
        queryClient.setQueriesData({ queryKey }, (data: unknown) => {
          if (!data || typeof data !== 'object' || !('items' in data) || !Array.isArray(data.items)) return data;
          const items = data.items.filter((record: RecordType) => identify(record) !== id);
          const removed = items.length !== data.items.length;
          const total = Number('total' in data ? data.total : data.items.length);
          return { ...data, items, total: removed ? Math.max(0, total - 1) : total };
        });
      }
      return { previous };
    },
    onError: (_error, _variables, context) => {
      context?.previous.forEach(([key, data]) => queryClient.setQueryData(key, data));
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });

  return { create, update, delete: remove };
}

export function subscribePocketBaseResource(
  pb: PocketBase,
  collectionName: string,
  onChange: (event: { action: string; record: unknown }) => void,
  onError: (error: unknown) => void = () => undefined,
) {
  const collection = pb.collection(collectionName);
  let active = true;
  let cancel: (() => void) | undefined;
  void collection.subscribe('*', (event) => {
    onChange({ action: event.action, record: event.record });
  }).then((unsubscribe) => {
    cancel = unsubscribe;
    if (!active) unsubscribe();
  }).catch(onError);
  return () => {
    active = false;
    if (cancel) cancel();
    else void collection.unsubscribe('*');
  };
}

/** Invalidates every cached query for a PocketBase-backed resource on realtime changes. */
export function usePocketBaseResourceRealtime(
  pb: PocketBase,
  resource: Pick<ResourceDefinition, 'id' | 'source'>,
  enabled = true,
) {
  const queryClient = useQueryClient();
  const collection = resource.source.kind === 'pocketbase' ? resource.source.collection : undefined;
  useEffect(() => {
    if (!enabled || !collection) return;
    return subscribePocketBaseResource(pb, collection, () => {
      void queryClient.invalidateQueries({ queryKey: resourceQueryKey(resource.id) });
    });
  }, [collection, enabled, pb, queryClient, resource.id]);
}
