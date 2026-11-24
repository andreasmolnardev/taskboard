import { useSyncExternalStore } from 'react';
import type { SettingsDefinition } from './types';

export type SettingsAdapter<Value extends Record<string, unknown>> = {
  load: () => Promise<Partial<Value> | null>;
  save: (value: Value) => Promise<void>;
};

export type SettingsSnapshot<Value extends Record<string, unknown>> = {
  value: Value;
  status: 'idle' | 'loading' | 'ready' | 'saving' | 'error';
  error?: unknown;
};

export type SettingsStore<Value extends Record<string, unknown>> = {
  getSnapshot: () => SettingsSnapshot<Value>;
  subscribe: (listener: () => void) => () => void;
  load: () => Promise<void>;
  update: (patch: Partial<Value>) => Promise<Value>;
  reset: () => Promise<Value>;
};

/** Creates a typed settings store over a caller-selected persistence adapter. */
export function createSettingsStore<Value extends Record<string, unknown>>(
  definition: SettingsDefinition<Value>,
  adapter: SettingsAdapter<Value>,
): SettingsStore<Value> {
  let snapshot: SettingsSnapshot<Value> = { value: { ...definition.defaults }, status: 'idle' };
  const listeners = new Set<() => void>();
  let loadPromise: Promise<void> | undefined;
  const publish = (next: SettingsSnapshot<Value>) => {
    snapshot = next;
    listeners.forEach((listener) => listener());
  };
  return {
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    load() {
      if (loadPromise) return loadPromise;
      publish({ ...snapshot, status: 'loading', error: undefined });
      loadPromise = adapter.load()
        .then((stored) => publish({ value: { ...definition.defaults, ...stored }, status: 'ready' }))
        .catch((error: unknown) => {
          publish({ ...snapshot, status: 'error', error });
          throw error;
        })
        .finally(() => { loadPromise = undefined; });
      return loadPromise;
    },
    async update(patch) {
      const next = { ...snapshot.value, ...patch };
      publish({ ...snapshot, status: 'saving', error: undefined });
      try {
        await adapter.save(next);
        publish({ value: next, status: 'ready' });
        return next;
      } catch (error) {
        publish({ ...snapshot, status: 'error', error });
        throw error;
      }
    },
    async reset() {
      const next = { ...definition.defaults };
      publish({ ...snapshot, status: 'saving', error: undefined });
      try {
        await adapter.save(next);
        publish({ value: next, status: 'ready' });
        return next;
      } catch (error) {
        publish({ ...snapshot, status: 'error', error });
        throw error;
      }
    },
  };
}

/** Safe browser persistence for local appearance or single-user preferences. */
export function createLocalStorageSettingsAdapter<Value extends Record<string, unknown>>(
  storageKey: string,
  storage: Pick<Storage, 'getItem' | 'setItem'> | undefined = typeof localStorage === 'undefined' ? undefined : localStorage,
): SettingsAdapter<Value> {
  return {
    async load() {
      const raw = storage?.getItem(storageKey);
      return raw ? JSON.parse(raw) as Partial<Value> : null;
    },
    async save(value) {
      if (!storage) throw new Error('Browser storage is unavailable; provide a settings adapter for this runtime.');
      storage.setItem(storageKey, JSON.stringify(value));
    },
  };
}

export function useSettings<Value extends Record<string, unknown>>(store: SettingsStore<Value>) {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
