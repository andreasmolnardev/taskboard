import { useSyncExternalStore } from 'react';
import type { BackgroundOperationDefinition } from './types';

export type OperationStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';
export type OperationSnapshot = {
  id: string;
  definitionId: string;
  label: string;
  status: OperationStatus;
  progress: number;
  message?: string;
  error?: unknown;
  startedAt: number;
  finishedAt?: number;
};

export type BackgroundOperationManager = {
  start: <Input, Output>(definition: BackgroundOperationDefinition<Input, Output>, input: Input) => string;
  cancel: (id: string) => boolean;
  remove: (id: string) => boolean;
  get: (id: string) => OperationSnapshot | undefined;
  list: () => readonly OperationSnapshot[];
  subscribe: (listener: () => void) => () => void;
};

/** Tracks client-side long-running work; server jobs can report progress through the same callbacks. */
export function createBackgroundOperationManager(): BackgroundOperationManager {
  const operations = new Map<string, OperationSnapshot>();
  const controllers = new Map<string, AbortController>();
  const listeners = new Set<() => void>();
  let listSnapshot: readonly OperationSnapshot[] = [];
  let sequence = 0;
  const publish = () => {
    listSnapshot = [...operations.values()];
    listeners.forEach((listener) => listener());
  };
  const patch = (id: string, updates: Partial<OperationSnapshot>) => {
    const current = operations.get(id);
    if (!current) return;
    operations.set(id, { ...current, ...updates });
    publish();
  };

  return {
    start(definition, input) {
      const id = definition.id + ':' + (++sequence);
      const controller = new AbortController();
      controllers.set(id, controller);
      operations.set(id, {
        id,
        definitionId: definition.id,
        label: definition.label,
        status: 'running',
        progress: 0,
        startedAt: Date.now(),
      });
      publish();
      Promise.resolve().then(() => {
        if (controller.signal.aborted) return;
        return definition.run(input, {
          signal: controller.signal,
          reportProgress(progress, message) {
            if (controller.signal.aborted) return;
            patch(id, { progress: Math.max(0, Math.min(100, progress)), message });
          },
        });
      }).then(() => {
        if (controller.signal.aborted) patch(id, { status: 'cancelled', finishedAt: Date.now() });
        else patch(id, { status: 'succeeded', progress: 100, finishedAt: Date.now() });
      }).catch((error: unknown) => {
        if (controller.signal.aborted) patch(id, { status: 'cancelled', finishedAt: Date.now() });
        else patch(id, { status: 'failed', error, finishedAt: Date.now() });
      }).finally(() => controllers.delete(id));
      return id;
    },
    cancel(id) {
      const controller = controllers.get(id);
      if (!controller || controller.signal.aborted) return false;
      controller.abort();
      patch(id, { status: 'cancelled', finishedAt: Date.now() });
      return true;
    },
    remove(id) {
      if (operations.get(id)?.status === 'running') return false;
      const removed = operations.delete(id);
      if (removed) publish();
      return removed;
    },
    get(id) { return operations.get(id); },
    list() { return listSnapshot; },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

export function useBackgroundOperations(manager: BackgroundOperationManager) {
  return useSyncExternalStore(manager.subscribe, manager.list, manager.list);
}
