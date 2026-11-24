import { useSyncExternalStore } from 'react';

export type StatePersistence<State> = {
  load: () => Promise<State | null>;
  save: (state: State) => Promise<void>;
};

export type SharedStateStore<State> = {
  getSnapshot: () => State;
  subscribe: (listener: () => void) => () => void;
  load: () => Promise<void>;
  set: (next: State) => Promise<void>;
  update: (updater: (current: State) => State) => Promise<void>;
};

/** A small cross-view state store with optional persistence and an adapter-neutral contract. */
export function createSharedStateStore<State>(
  initial: State,
  persistence?: StatePersistence<State>,
): SharedStateStore<State> {
  let snapshot = initial;
  const listeners = new Set<() => void>();
  let loadPromise: Promise<void> | undefined;
  let writeQueue: Promise<void> = Promise.resolve();
  const publish = (next: State) => {
    snapshot = next;
    listeners.forEach((listener) => listener());
  };
  const enqueue = (operation: () => Promise<void>) => {
    const next = writeQueue.then(operation);
    writeQueue = next.catch(() => undefined);
    return next;
  };
  return {
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    load() {
      if (!persistence) return Promise.resolve();
      if (loadPromise) return loadPromise;
      loadPromise = persistence.load().then((stored) => {
        if (stored !== null) publish(stored);
      }).finally(() => { loadPromise = undefined; });
      return loadPromise;
    },
    set(next) {
      return enqueue(async () => {
        await persistence?.save(next);
        publish(next);
      });
    },
    update(updater) {
      return enqueue(async () => {
        const next = updater(snapshot);
        await persistence?.save(next);
        publish(next);
      });
    },
  };
}

export function useSharedState<State>(store: SharedStateStore<State>) {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
