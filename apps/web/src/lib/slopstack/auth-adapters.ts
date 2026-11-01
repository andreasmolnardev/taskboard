import PocketBase from 'pocketbase';
import type { AuthSession, AuthUser, AuthenticationAdapter } from './types';

export type PocketBaseCredentials = { identity: string; password: string };

function pocketBaseSession(pb: PocketBase): AuthSession | null {
  const record = pb.authStore.record;
  if (!pb.authStore.isValid || !record) return null;
  const user = { ...record } as AuthUser;
  return { user, accessToken: pb.authStore.token };
}

/** Adapts PocketBase auth state to the provider-neutral Slopstack auth contract. */
export function createPocketBaseAuthAdapter(
  pb: PocketBase,
  collection = 'users',
): AuthenticationAdapter<PocketBaseCredentials> {
  return {
    async getSession() { return pocketBaseSession(pb); },
    subscribe(listener) {
      return pb.authStore.onChange(() => listener(pocketBaseSession(pb)), true);
    },
    async signIn({ identity, password }) {
      await pb.collection(collection).authWithPassword(identity, password);
      return pocketBaseSession(pb);
    },
    async signOut() { pb.authStore.clear(); },
  };
}

export type TrustedProxyAuthOptions = {
  sessionEndpoint: string;
  signInUrl: string;
  signOutEndpoint?: string;
  signOutUrl?: string;
  fetcher?: typeof fetch;
  navigate?: (url: string) => void;
};

/** Reads identity only from a server endpoint backed by trusted proxy middleware. */
export function createTrustedProxyAuthAdapter(
  options: TrustedProxyAuthOptions,
): AuthenticationAdapter<void> {
  const fetcher = options.fetcher ?? fetch;
  const readSession = async (): Promise<AuthSession | null> => {
    const response = await fetcher(options.sessionEndpoint, { credentials: 'include' });
    if (response.status === 401 || response.status === 403) return null;
    if (!response.ok) throw new Error(`Could not read session (HTTP ${response.status}).`);
    const result = await response.json() as { user?: AuthUser; expiresAt?: number };
    if (!result.user?.id) throw new Error('The session endpoint returned no authenticated user.');
    return { user: result.user, expiresAt: result.expiresAt };
  };
  return {
    getSession: readSession,
    subscribe(listener) {
      if (typeof window === 'undefined') return () => undefined;
      let active = true;
      const refresh = () => { void readSession().then((session) => { if (active) listener(session); }).catch(() => { if (active) listener(null); }); };
      window.addEventListener('focus', refresh);
      document.addEventListener('visibilitychange', refresh);
      return () => {
        active = false;
        window.removeEventListener('focus', refresh);
        document.removeEventListener('visibilitychange', refresh);
      };
    },
    async signIn() {
      const navigate = options.navigate ?? ((url: string) => window.location.assign(url));
      navigate(options.signInUrl);
      return null;
    },
    async signOut() {
      if (options.signOutEndpoint) {
        const response = await fetcher(options.signOutEndpoint, { method: 'POST', credentials: 'include' });
        if (!response.ok) throw new Error(`Could not sign out (HTTP ${response.status}).`);
        return;
      }
      if (options.signOutUrl) {
        const navigate = options.navigate ?? ((url: string) => window.location.assign(url));
        navigate(options.signOutUrl);
        return;
      }
      throw new Error('Trusted-proxy auth needs a sign-out endpoint or URL.');
    },
  };
}

export type BearerLoginResponse = { token: string; user: AuthUser; expiresAt?: number };

export type BearerAuthOptions = {
  loginEndpoint: string;
  sessionEndpoint: string;
  signOutEndpoint?: string;
  getToken: () => string | null;
  setToken: (token: string | null) => void;
  fetcher?: typeof fetch;
};

/** Adapts a JSON login API and bearer-token session endpoint without choosing token storage for the app. */
export function createBearerAuthAdapter<Credentials>(
  options: BearerAuthOptions,
): AuthenticationAdapter<Credentials> {
  const fetcher = options.fetcher ?? fetch;
  const listeners = new Set<(session: AuthSession | null) => void>();
  const notify = (session: AuthSession | null) => listeners.forEach((listener) => listener(session));
  const readSession = async (): Promise<AuthSession | null> => {
    const token = options.getToken();
    if (!token) return null;
    const response = await fetcher(options.sessionEndpoint, { headers: { Authorization: `Bearer ${token}` } });
    if (response.status === 401 || response.status === 403) return null;
    if (!response.ok) throw new Error(`Could not read session (HTTP ${response.status}).`);
    const result = await response.json() as { user?: AuthUser; expiresAt?: number };
    if (!result.user?.id) throw new Error('The session endpoint returned no authenticated user.');
    return { user: result.user, accessToken: token, expiresAt: result.expiresAt };
  };
  return {
    getSession: readSession,
    subscribe(listener) {
      listeners.add(listener);
      if (typeof window !== 'undefined') {
        const onFocus = () => { void readSession().then(notify).catch(() => undefined); };
        window.addEventListener('focus', onFocus);
        return () => { listeners.delete(listener); window.removeEventListener('focus', onFocus); };
      }
      return () => { listeners.delete(listener); };
    },
    async signIn(credentials) {
      const response = await fetcher(options.loginEndpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(credentials),
      });
      if (!response.ok) throw new Error((await response.text().catch(() => '')) || `Sign-in failed (HTTP ${response.status}).`);
      const result = await response.json() as BearerLoginResponse;
      if (!result.token || !result.user?.id) throw new Error('The sign-in endpoint returned an invalid session.');
      options.setToken(result.token);
      const session = { user: result.user, accessToken: result.token, expiresAt: result.expiresAt };
      notify(session);
      return session;
    },
    async signOut() {
      const token = options.getToken();
      if (token && options.signOutEndpoint) {
        const response = await fetcher(options.signOutEndpoint, { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
        if (!response.ok) throw new Error(`Could not sign out (HTTP ${response.status}).`);
      }
      options.setToken(null);
      notify(null);
    },
  };
}
