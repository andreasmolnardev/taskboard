import type { SyncStatus } from '../../offline';

export type SyncTone = 'synced' | 'syncing' | 'pending' | 'unreachable';

export function getSyncTone(status: SyncStatus): SyncTone {
  if (status.syncing) return 'syncing';
  if (!status.online && status.pending > 0) return 'pending';
  if (!status.online || status.error || status.paused || status.conflicts > 0) return 'unreachable';
  if (status.pending > 0 || status.lastSyncedAt === null) return 'pending';
  return 'synced';
}
