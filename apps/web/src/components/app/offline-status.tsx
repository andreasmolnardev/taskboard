import { useEffect, useState } from 'react';
import { pb } from '../../api/pocketbase';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../ui/dialog';
import {
  OFFLINE_ENABLED,
  getOfflineConflicts,
  getOfflineStatus,
  resolveConflict,
  retryOfflineSync,
  subscribeOffline,
} from '../../offline';
import { getSyncTone, type SyncTone } from './sync-tone';

function getSyncLabel(tone: SyncTone) {
  switch (tone) {
    case 'synced':
      return 'Synced successfully';
    case 'syncing':
      return 'Syncing';
    case 'pending':
      return 'Sync pending';
    case 'unreachable':
      return 'Out of sync';
  }
}

function fieldLabel(field: string) {
  return field.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function fieldValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return 'Empty';
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

export function OfflineStatus() {
  const [status, setStatus] = useState(getOfflineStatus);
  const [conflicts, setConflicts] = useState<Awaited<ReturnType<typeof getOfflineConflicts>>>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [password, setPassword] = useState('');
  const [open, setOpen] = useState(false);
  const [providers, setProviders] = useState<Array<{ name: string; displayName: string }>>([]);

  useEffect(() => {
    if (!status.paused) return;
    let active = true;
    void pb
      .collection('users')
      .listAuthMethods()
      .then((methods) => {
        if (active) setProviders(methods.oauth2.providers);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [status.paused]);

  useEffect(() => {
    const unsubscribe = subscribeOffline(() => setStatus(getOfflineStatus()));
    return unsubscribe;
  }, []);

  useEffect(() => {
    let active = true;
    void getOfflineConflicts().then((items) => {
      if (active) setConflicts(items);
    });
    return () => {
      active = false;
    };
  }, [status]);

  async function resolve(id: string, choice: 'local' | 'remote') {
    setBusy(true);
    setError('');
    try {
      await resolveConflict(id, choice);
      setConflicts(await getOfflineConflicts());
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Could not save conflict choice.');
    } finally {
      setBusy(false);
    }
  }

  if (!OFFLINE_ENABLED) return null;

  const tone = getSyncTone(status);
  const label = getSyncLabel(tone);
  const lastSynced = status.lastSyncedAt
    ? new Date(status.lastSyncedAt).toLocaleString()
    : 'Not synced yet';

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          type="button"
          className={`sync-status-trigger sync-status-${tone}`}
          aria-label={`Sync status: ${label}`}
          title={label}
        >
          <span className="sync-status-dot" aria-hidden="true" />
        </button>
      </DialogTrigger>
      <DialogContent className="sync-status-dialog max-h-[min(85vh,48rem)] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Sync status</DialogTitle>
          <DialogDescription>{label}</DialogDescription>
        </DialogHeader>

        <dl className="sync-status-details">
          <div>
            <dt>Connection</dt>
            <dd>{status.online ? 'Online' : 'Offline — server unreachable'}</dd>
          </div>
          <div>
            <dt>Pending changes</dt>
            <dd>{status.pending}</dd>
          </div>
          <div>
            <dt>Conflicts</dt>
            <dd>{status.conflicts}</dd>
          </div>
          <div>
            <dt>Last successful sync</dt>
            <dd>{lastSynced}</dd>
          </div>
        </dl>

        {status.error && <p className="form-error">{status.error}</p>}
        {status.paused && !import.meta.env.DEV && (
          <form
            className="offline-auth-form"
            onSubmit={(event) => {
              event.preventDefault();
              setBusy(true);
              setError('');
              void pb
                .collection('users')
                .authWithPassword(String(pb.authStore.record?.email ?? ''), password)
                .then(() => {
                  setPassword('');
                  retryOfflineSync();
                })
                .catch(() => setError('Could not sign in. Check your password and connection.'))
                .finally(() => setBusy(false));
            }}
          >
            <p>Sign in again to sync. Pending edits stay on this device.</p>
            <label>
              Password{' '}
              <input
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
            </label>{' '}
            <button className="button" disabled={busy || !status.online}>
              Resume sync
            </button>
            {providers.map((provider) => (
              <button
                type="button"
                className="button"
                key={provider.name}
                disabled={busy || !status.online}
                onClick={() => {
                  setBusy(true);
                  setError('');
                  void pb
                    .collection('users')
                    .authWithOAuth2({ provider: provider.name })
                    .then(() => retryOfflineSync())
                    .catch(() => setError('Could not sign in with this provider.'))
                    .finally(() => setBusy(false));
                }}
              >
                Sign in with {provider.displayName}
              </button>
            ))}
          </form>
        )}

        <button
          type="button"
          className="button"
          disabled={status.syncing || !status.online}
          onClick={() => void retryOfflineSync()}
        >
          {status.syncing ? 'Syncing…' : 'Sync now'}
        </button>

        {conflicts.map((conflict) => (
          <details key={conflict.operationId}>
            <summary>
              Resolve change to{' '}
              {String(
                conflict.remote?.title ??
                  conflict.remote?.name ??
                  conflict.patch.title ??
                  conflict.patch.name ??
                  'entry',
              )}
            </summary>
            <p>{conflict.message}</p>
            {conflict.kind === 'delete' && <p>Your change deletes this entry.</p>}
            {conflict.deleted && <p>The entry was deleted on the server.</p>}
            <div className="offline-conflict-values">
              {Object.entries(conflict.patch)
                .filter(
                  ([field]) =>
                    ![
                      'id',
                      'owner',
                      'collectionId',
                      'collectionName',
                      'created',
                      'updated',
                      'dtstamp',
                    ].includes(field),
                )
                .map(([field, value]) => (
                  <div key={field}>
                    <strong>{fieldLabel(field)}</strong>
                    <p>Your edit: {fieldValue(value)}</p>
                    <p>Server: {fieldValue(conflict.remote?.[field])}</p>
                  </div>
                ))}
            </div>
            <button
              className="button"
              disabled={busy}
              onClick={() => void resolve(conflict.operationId, 'local')}
            >
              Keep my edit
            </button>{' '}
            <button
              className="button"
              disabled={busy}
              onClick={() => void resolve(conflict.operationId, 'remote')}
            >
              Use server version
            </button>
          </details>
        ))}
        {error && (
          <p role="alert" className="form-error">
            {error}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
