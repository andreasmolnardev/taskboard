import { useEffect, useState, type FormEvent } from 'react';
import { Link } from '@tanstack/react-router';
import { Check } from 'lucide-react';
import { apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';

export function AuthScreen({ mode }: { mode: 'login' | 'register' }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [registrationMode, setRegistrationMode] = useState<'disabled' | 'approval' | 'otp'>(
    'approval',
  );
  const [ssoProviders, setSsoProviders] = useState<{ name: string; displayName: string }[]>([]);
  const [ssoBusy, setSsoBusy] = useState(false);
  useEffect(() => {
    if (mode !== 'login') return;
    void pb
      .collection('users')
      .listAuthMethods()
      .then((methods) => {
        if (methods.oauth2.enabled) {
          setSsoProviders(
            methods.oauth2.providers.map(({ name, displayName }) => ({ name, displayName })),
          );
        }
      })
      .catch(() => setSsoProviders([]));
  }, [mode]);
  useEffect(() => {
    if (mode !== 'register') return;
    void apiFetch('/api/auth/registration-policy')
      .then((response) => response.json())
      .then((policy: { mode: 'disabled' | 'approval' | 'otp' }) => setRegistrationMode(policy.mode))
      .catch(() => setError('Could not load registration settings.'));
  }, [mode]);
  const handleSso = async (provider: string) => {
    setSsoBusy(true);
    setError('');
    try {
      await pb.collection('users').authWithOAuth2({ provider });
    } catch {
      setError('Could not sign in with SSO. Try again.');
    } finally {
      setSsoBusy(false);
    }
  };
  const handleGuestAuth = async () => {
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const response = await apiFetch('/api/auth/guest', { method: 'POST' });
      if (!response.ok) throw new Error('Guest authentication failed');
      const result: {
        token: string;
        record: {
          id: string;
          collectionId: string;
          collectionName: string;
          [key: string]: unknown;
        };
      } = await response.json();
      pb.authStore.save(result.token, result.record);
    } catch {
      setError('Could not authenticate as a guest.');
    } finally {
      setBusy(false);
    }
  };
  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    setMessage('');
    try {
      if (mode === 'register') {
        if (registrationMode === 'disabled') return;
        await pb.collection('users').create({ email, password, passwordConfirm: password });
        setMessage(
          registrationMode === 'otp'
            ? 'Account created. Read the activation code in the server logs, then ask an administrator to activate your account.'
            : 'Account created. An administrator must approve it before you can sign in.',
        );
        return;
      }
      await pb.collection('users').authWithPassword(email, password);
    } catch {
      setError(
        mode === 'register'
          ? 'Could not create your account. Check your details.'
          : 'Could not sign in. Check your email and password.',
      );
    } finally {
      setBusy(false);
    }
  };
  const disabled = mode === 'register' && registrationMode === 'disabled';
  return (
    <main className="auth-screen">
      <form className="auth-card" onSubmit={handleSubmit}>
        <div className="sidebar-brand">
          <Check size={22} />
        </div>
        <h1>{mode === 'register' ? 'Create your account' : 'Welcome back'}</h1>
        {mode === 'login' && <p>Sign in to see your Taskboard.</p>}
        {message && <div className="auth-message">{message}</div>}
        {!disabled && (
          <>
            <label>
              Email
              <input
                type="email"
                required
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
              />
            </label>
            <label>
              Password
              <input
                type="password"
                required
                autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </label>
          </>
        )}
        {error && <div className="auth-error">{error}</div>}
        {!disabled && (
          <button className="button button-primary auth-submit" disabled={busy || ssoBusy}>
            {busy
              ? mode === 'register'
                ? 'Creating account…'
                : 'Signing in…'
              : mode === 'register'
                ? 'Create account'
                : 'Sign in'}
          </button>
        )}
        {mode === 'login' && ssoProviders.length > 0 && (
          <div className="sso-options">
            <div className="sso-divider">or</div>
            {ssoProviders.map((provider) => (
              <button
                key={provider.name}
                type="button"
                className="button auth-sso"
                disabled={busy || ssoBusy}
                onClick={() => void handleSso(provider.name)}
              >
                {ssoBusy ? 'Opening SSO…' : `Sign in with ${provider.displayName}`}
              </button>
            ))}
          </div>
        )}
        <p className="auth-switch">
          {mode === 'register' ? 'Already have an account?' : 'Need an account?'}{' '}
          <Link to={mode === 'register' ? '/auth/login' : '/auth/register'}>
            {mode === 'register' ? 'Sign in' : 'Sign up'}
          </Link>
        </p>
        {mode === 'login' && import.meta.env.DEV && (
          <button
            type="button"
            className="button auth-guest"
            disabled={busy}
            onClick={() => void handleGuestAuth()}
          >
            {busy ? 'Authenticating…' : 'Authenticate as guest'}
          </button>
        )}
      </form>
    </main>
  );
}
