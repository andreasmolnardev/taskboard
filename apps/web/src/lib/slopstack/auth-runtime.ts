import { useSyncExternalStore } from 'react';
import type { AuthSession, AuthenticationAdapter } from './types';

export type AuthenticationSnapshot = {
  session: AuthSession | null;
  status: 'idle' | 'checking' | 'anonymous' | 'authenticated' | 'signing-in' | 'signing-out' | 'error';
  error?: unknown;
};

export type AuthenticationStore<Credentials> = {
  getSnapshot: () => AuthenticationSnapshot;
  subscribe: (listener: () => void) => () => void;
  initialize: () => Promise<void>;
  signIn: (credentials: Credentials) => Promise<AuthSession | null>;
  signOut: () => Promise<void>;
  dispose: () => void;
};

/** Coordinates session state without prescribing a login form or credential type. */
export function createAuthenticationStore<Credentials>(
  adapter: AuthenticationAdapter<Credentials>,
): AuthenticationStore<Credentials> {
  let snapshot: AuthenticationSnapshot = { session: null, status: 'idle' };
  const listeners = new Set<() => void>();
  let subscription: (() => void) | undefined;
  let initializePromise: Promise<void> | undefined;
  const publish = (next: AuthenticationSnapshot) => {
    snapshot = next;
    listeners.forEach((listener) => listener());
  };
  const receiveSession = (session: AuthSession | null) => publish({
    session,
    status: session ? 'authenticated' : 'anonymous',
  });

  return {
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    initialize() {
      if (initializePromise) return initializePromise;
      if (!subscription) subscription = adapter.subscribe(receiveSession);
      publish({ ...snapshot, status: 'checking', error: undefined });
      initializePromise = adapter.getSession()
        .then(receiveSession)
        .catch((error: unknown) => publish({ session: null, status: 'error', error }))
        .finally(() => { initializePromise = undefined; });
      return initializePromise;
    },
    async signIn(credentials) {
      publish({ ...snapshot, status: 'signing-in', error: undefined });
      try {
        const session = await adapter.signIn(credentials);
        receiveSession(session);
        return session;
      } catch (error) {
        publish({ ...snapshot, status: 'error', error });
        throw error;
      }
    },
    async signOut() {
      publish({ ...snapshot, status: 'signing-out', error: undefined });
      try {
        await adapter.signOut();
        receiveSession(null);
      } catch (error) {
        publish({ ...snapshot, status: 'error', error });
        throw error;
      }
    },
    dispose() {
      subscription?.();
      subscription = undefined;
      listeners.clear();
    },
  };
}

export function useAuthentication<Credentials>(store: AuthenticationStore<Credentials>) {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
