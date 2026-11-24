import { describe, expect, it } from 'vitest';
import type { SyncStatus } from '../../offline';
import { getSyncTone } from './sync-tone';

function status(overrides: Partial<SyncStatus> = {}): SyncStatus {
  return {
    online: true,
    syncing: false,
    paused: false,
    pending: 0,
    conflicts: 0,
    lastSyncedAt: 1,
    error: null,
    ...overrides,
  };
}

describe('sync status indicator', () => {
  it('shows green when synced successfully', () => {
    expect(getSyncTone(status())).toBe('synced');
  });

  it('shows blue while syncing', () => {
    expect(getSyncTone(status({ syncing: true, pending: 1 }))).toBe('syncing');
  });

  it('shows yellow when changes are pending, even while offline', () => {
    expect(getSyncTone(status({ online: false, pending: 2 }))).toBe('pending');
    expect(getSyncTone(status({ pending: 2 }))).toBe('pending');
    expect(getSyncTone(status({ lastSyncedAt: null }))).toBe('pending');
  });

  it('shows red when unreachable or blocked from syncing', () => {
    expect(getSyncTone(status({ online: false }))).toBe('unreachable');
    expect(getSyncTone(status({ error: 'Network request failed' }))).toBe('unreachable');
    expect(getSyncTone(status({ paused: true }))).toBe('unreachable');
    expect(getSyncTone(status({ conflicts: 1 }))).toBe('unreachable');
  });
});
