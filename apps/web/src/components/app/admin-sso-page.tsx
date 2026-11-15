import { useEffect, useState, type FormEvent } from 'react';
import { pb } from '../../api/pocketbase';

type OidcProvider = {
  name: string;
  clientId?: string;
  authURL?: string;
  tokenURL?: string;
  userInfoURL?: string;
  extra?: Record<string, unknown>;
};

export function AdminSsoPage() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [provider, setProvider] = useState<OidcProvider>({ name: 'oidc2' });
  const [otherProviders, setOtherProviders] = useState<OidcProvider[]>([]);
  const [secret, setSecret] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const isSuperuser = pb.authStore.record?.collectionName === '_superusers';

  async function load() {
    const collection = await pb.send('/api/collections/users', { method: 'GET' }) as {
      oauth2?: { enabled?: boolean; providers?: OidcProvider[] };
    };
    const current = collection.oauth2?.providers?.find((item) => item.name === 'oidc2');
    setEnabled(Boolean(collection.oauth2?.enabled));
    setProvider(current ?? { name: 'oidc2' });
    setOtherProviders(collection.oauth2?.providers?.filter((item) => item.name !== 'oidc2') ?? []);
  }

  useEffect(() => {
    if (isSuperuser) void load().catch(() => setError('Could not load PocketBase OAuth settings.'));
  }, [isSuperuser]);

  async function signIn(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      await pb.collection('_superusers').authWithPassword(email, password);
      await load();
    } catch {
      setError('Superuser sign-in failed.');
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await pb.send('/api/collections/users', {
        method: 'PATCH',
        body: {
          oauth2: {
            enabled,
            providers: [
              ...otherProviders,
              ...(provider.clientId ? [{ ...provider, ...(secret ? { clientSecret: secret } : {}) }] : []),
            ],
          },
        },
      });
      setSecret('');
      setMessage('OIDC settings saved.');
    } catch {
      setError('Could not save OIDC settings. Check the fields and superuser access.');
    } finally {
      setBusy(false);
    }
  }

  async function discover() {
    try {
      const issuer = String(provider.extra?.issuers instanceof Array ? provider.extra.issuers[0] ?? '' : '').trim().replace(/\/$/, '');
      const url = new URL(issuer);
      if (url.protocol !== 'https:' && url.hostname !== 'localhost') throw new Error('Issuer must use HTTPS.');
      const response = await fetch(`${issuer}/.well-known/openid-configuration`);
      if (!response.ok) throw new Error('Discovery endpoint returned an error.');
      const meta = await response.json() as Record<string, string>;
      setProvider((current) => ({
        ...current,
        authURL: meta.authorization_endpoint,
        tokenURL: meta.token_endpoint,
        userInfoURL: meta.userinfo_endpoint,
        extra: { ...current.extra, issuers: [meta.issuer || issuer], jwksURL: meta.jwks_uri },
      }));
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'OIDC discovery failed.');
    }
  }

  if (!isSuperuser) return <main className="auth-screen"><form className="auth-card" onSubmit={(event) => void signIn(event)}>
    <h1>SSO administration</h1><p>Sign in with a PocketBase superuser account.</p>
    <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
    <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
    {error && <p className="auth-error" role="alert">{error}</p>}
    <button className="button button-primary" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form></main>;

  return <main className="auth-screen"><form className="auth-card" onSubmit={(event) => void save(event)}>
    <h1>Custom OIDC provider</h1><p>Manage Taskboard sign-in through PocketBase. Callback URL: <code>{pb.baseURL}/api/oauth2-redirect</code></p>
    <label><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /> Enable OAuth sign-in</label>
    <label>Client ID<input value={provider.clientId ?? ''} onChange={(event) => setProvider({ ...provider, clientId: event.target.value })} /></label>
    <label>Client secret<input type="password" autoComplete="new-password" value={secret} placeholder="Leave blank to keep the saved secret" onChange={(event) => setSecret(event.target.value)} /></label>
    <label>Issuer<input type="url" value={String(provider.extra?.issuers instanceof Array ? provider.extra.issuers[0] ?? '' : '')} onChange={(event) => setProvider({ ...provider, extra: { ...provider.extra, issuers: [event.target.value] } })} /></label>
    <button type="button" className="button" onClick={() => void discover()}>Load discovery metadata</button>
    <label>Authorization URL<input type="url" value={provider.authURL ?? ''} onChange={(event) => setProvider({ ...provider, authURL: event.target.value })} /></label>
    <label>Token URL<input type="url" value={provider.tokenURL ?? ''} onChange={(event) => setProvider({ ...provider, tokenURL: event.target.value })} /></label>
    <label>User info URL<input type="url" value={provider.userInfoURL ?? ''} onChange={(event) => setProvider({ ...provider, userInfoURL: event.target.value })} /></label>
    <label>JWKS URL<input type="url" value={String(provider.extra?.jwksURL ?? '')} onChange={(event) => setProvider({ ...provider, extra: { ...provider.extra, jwksURL: event.target.value } })} /></label>
    {error && <p className="auth-error" role="alert">{error}</p>}{message && <p className="auth-message">{message}</p>}
    <button className="button button-primary" disabled={busy}>{busy ? 'Saving…' : 'Save settings'}</button>
    <button type="button" className="button" onClick={() => { pb.authStore.clear(); window.location.assign('/auth/login'); }}>Sign out</button>
  </form></main>;
}
