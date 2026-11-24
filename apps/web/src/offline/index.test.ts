import 'fake-indexeddb/auto';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({
  token: '',
  record: { id: 'offline-test-user' } as { id: string } | null,
}));
vi.mock('../api/pocketbase', () => ({
  pb: {
    authStore: auth,
    collection: () => ({ subscribe: async () => undefined, unsubscribe: async () => undefined }),
  },
}));
vi.mock('../api/client', () => ({ apiBaseUrl: 'http://taskboard.test', apiFetch: vi.fn() }));

import { apiFetch } from '../api/client';
import {
  getOfflineConflicts,
  getPendingRecurrenceActions,
  mergeRemoteWithLocal,
  offlineCollection,
  offlineJson,
  offlineRecurrence,
  retryOfflineSync,
  resolveConflict,
  startOffline,
  stopOffline,
  waitForOfflineRecord,
} from './index';

function response(payload: unknown, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => payload } as Response;
}

describe('offline repository', () => {
  beforeEach(() => {
    vi.stubGlobal('navigator', { onLine: false });
    vi.stubGlobal('window', {
      addEventListener() {},
      removeEventListener() {},
      setTimeout,
      clearTimeout,
    });
    auth.token = '';
    auth.record = { id: 'offline-test-user' };
    vi.mocked(apiFetch).mockReset();
  });
  afterEach(async () => {
    await stopOffline(true);
    vi.unstubAllGlobals();
  });

  it('stores local create, update, and delete durably in IndexedDB', async () => {
    await startOffline('offline-test-user');
    const todos = offlineCollection<{ id: string; title: string; done?: boolean }>('todos');
    const created = await todos.create({ title: 'Plan the week' });
    expect(created.id).toMatch(/^[a-z0-9]{15}$/);
    expect(await todos.getFullList()).toEqual([created]);
    await todos.update(created.id, { done: true });
    expect(await todos.getOne(created.id)).toMatchObject({ title: 'Plan the week', done: true });
    await stopOffline(false);
    await startOffline('offline-test-user');
    expect(await todos.getFullList()).toMatchObject([
      { id: created.id, title: 'Plan the week', done: true },
    ]);
    await todos.delete(created.id);
    expect(await todos.getFullList()).toEqual([]);
  });

  it('merges independent fields and reports same-field conflicts', () => {
    const base = { id: '1', title: 'Old', description: 'A', status: 'todo', completed: false };
    const local = { ...base, title: 'Local' };
    const remote = { ...base, description: 'Remote' };
    expect(mergeRemoteWithLocal(base, local, remote)).toEqual({
      merged: { ...remote, title: 'Local' },
      conflicts: [],
    });
    const linkedRemote = { ...remote, completed: true };
    const linkedLocal = { ...base, status: 'done' };
    expect(mergeRemoteWithLocal(base, linkedLocal, linkedRemote).conflicts[0]).toContain(
      'completed,status',
    );
  });

  it('pulls a base, pushes edits, and keeps edits made while a push is in flight', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    const base = {
      id: 'todo00000000001',
      title: 'First',
      description: '',
      owner: 'offline-test-user',
    };
    let serverRecord = { ...base };
    let revision = 1;
    let resolveFirst!: (value: Response) => void;
    let holdFirstPush = true;
    let firstOperation:
      { operationId: string; patch: Record<string, unknown>; baseRevision: number } | undefined;
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (String(path).includes('/pull?'))
        return response({
          cursor: revision,
          changes:
            revision === 1
              ? [{ collection: 'todos', id: base.id, revision: 1, deleted: false, record: base }]
              : [],
        });
      const operations = JSON.parse(String(init?.body)).operations;
      if (holdFirstPush) {
        firstOperation = operations[0];
        return new Promise<Response>((resolve) => {
          resolveFirst = resolve;
          holdFirstPush = false;
        });
      }
      const operation = operations[0];
      revision++;
      const canonical = { ...serverRecord, ...operation.patch };
      serverRecord = canonical;
      return response({
        results: [
          { operationId: operation.operationId, status: 'applied', revision, record: canonical },
        ],
      });
    });
    await startOffline('offline-test-user');
    auth.token = 'token';
    retryOfflineSync();
    const todos = offlineCollection('todos');
    await vi.waitFor(async () => expect(await todos.getOne(base.id)).toEqual(base));
    await todos.update(base.id, { title: 'Edit one' });
    await vi.waitFor(() => expect(firstOperation).toBeDefined());
    await todos.update(base.id, { description: 'Edit two' });
    serverRecord = { ...serverRecord, ...firstOperation!.patch };
    resolveFirst(
      response({
        results: [
          {
            operationId: firstOperation!.operationId,
            status: 'applied',
            revision: 2,
            record: serverRecord,
          },
        ],
      }),
    );
    await vi.waitFor(() =>
      expect(
        vi.mocked(apiFetch).mock.calls.filter(([, init]) => init?.method === 'POST'),
      ).toHaveLength(2),
    );
    expect(await todos.getOne(base.id)).toMatchObject({
      title: 'Edit one',
      description: 'Edit two',
    });
    const pushes = vi.mocked(apiFetch).mock.calls.filter(([, init]) => init?.method === 'POST');
    expect(JSON.parse(String(pushes[1]?.[1]?.body)).operations[0].patch).toEqual({
      description: 'Edit two',
    });
    expect(firstOperation!.baseRevision).toBe(1);
  });

  it('keeps recurrence operation payload intact when a pull sees the unchanged master revision', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    const event = {
      id: 'event0000000001',
      title: 'Weekly',
      rrule: 'FREQ=WEEKLY',
      owner: 'offline-test-user',
    };
    let pushed:
      | {
          operationId: string;
          collection: string;
          id: string;
          kind: string;
          baseRevision: number;
          patch: Record<string, unknown>;
        }
      | undefined;
    let pulled = false;
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (init?.method === 'POST') {
        pushed = JSON.parse(String(init.body)).operations[0];
        return response({
          results: [
            {
              operationId: pushed?.operationId,
              status: 'applied',
              revision: 2,
              record: {
                id: 'override0000001',
                recurrence_parent: event.id,
                recurrence_id: '2026-11-10T10:00:00Z',
                title: 'Moved',
              },
            },
          ],
        });
      }
      if (pulled) return response({ cursor: 1, changes: [] });
      pulled = true;
      return response({
        cursor: 1,
        changes: [
          { collection: 'events', id: event.id, revision: 1, deleted: false, record: event },
        ],
      });
    });
    await startOffline('offline-test-user');
    await vi.waitFor(async () =>
      expect(await offlineCollection('events').getOne(event.id)).toEqual(event),
    );
    const opId = await offlineRecurrence(event.id, 'override', '2026-11-10T10:00:00Z', {
      title: 'Moved',
    });
    const { getOfflineConflicts } = await import('./index');
    await vi.waitFor(() => expect(pushed).toBeDefined());
    expect(pushed).toMatchObject({
      operationId: opId,
      collection: 'events',
      id: event.id,
      kind: 'override',
      baseRevision: 1,
      patch: { recurrenceId: '2026-11-10T10:00:00Z', values: { title: 'Moved' } },
    });
    expect(await getOfflineConflicts()).toEqual([]);
    expect(opId).toBeTruthy();
  });

  it('retries an attempted operation with the same ID and payload after a lost response', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    const base = {
      id: 'todo00000000002',
      title: 'Original',
      description: '',
      owner: 'offline-test-user',
    };
    let serverRecord = { ...base };
    let serverRevision = 1;
    let pulledSnapshot = false;
    let firstRequest: { operationId: string; patch: Record<string, unknown> } | undefined;
    let retriedRequest: { operationId: string; patch: Record<string, unknown> } | undefined;
    let didDrop = false;
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (String(path).includes('/pull?')) {
        if (!pulledSnapshot) {
          pulledSnapshot = true;
          return response({
            cursor: 1,
            changes: [
              { collection: 'todos', id: base.id, revision: 1, deleted: false, record: base },
            ],
          });
        }
        if (serverRevision === 2)
          return response({
            cursor: 2,
            changes: [
              {
                collection: 'todos',
                id: base.id,
                revision: 2,
                deleted: false,
                record: serverRecord,
              },
            ],
          });
        return response({ cursor: serverRevision, changes: [] });
      }
      const operation = JSON.parse(String(init?.body)).operations[0];
      if (!firstRequest) {
        firstRequest = operation;
        serverRecord = { ...serverRecord, ...operation.patch };
        serverRevision = 2;
        didDrop = true;
        throw new Error('Connection lost after server applied the operation');
      }
      if (!retriedRequest) retriedRequest = operation;
      serverRecord = { ...serverRecord, ...operation.patch };
      serverRevision++;
      return response({
        results: [
          {
            operationId: operation.operationId,
            status: 'applied',
            revision: serverRevision,
            record: serverRecord,
          },
        ],
      });
    });
    await startOffline('offline-test-user');
    const todos = offlineCollection('todos');
    await vi.waitFor(async () => expect(await todos.getOne(base.id)).toEqual(base));
    await todos.update(base.id, { title: 'First edit' });
    await vi.waitFor(() => expect(didDrop).toBe(true));
    await todos.update(base.id, { description: 'Second edit' });
    retryOfflineSync();
    await vi.waitFor(() => expect(retriedRequest).toEqual(firstRequest));
    expect(await todos.getOne(base.id)).toMatchObject({
      title: 'First edit',
      description: 'Second edit',
    });
  });

  it('continues a snapshot pull when a page has exactly 500 changes', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    let page = 0;
    vi.mocked(apiFetch).mockImplementation(async (path) => {
      if (!String(path).includes('/pull?')) return response({ results: [] });
      page++;
      if (page === 1) {
        const changes = Array.from({ length: 500 }, (_, index) => {
          const id = `todo${String(index).padStart(11, '0')}`;
          return {
            collection: 'todos',
            id,
            revision: index + 1,
            deleted: false,
            record: { id, title: `Task ${index}` },
          };
        });
        return response({ cursor: 500, changes });
      }
      if (page === 2)
        return response({
          cursor: 501,
          changes: [
            {
              collection: 'todos',
              id: 'todo00000000500',
              revision: 501,
              deleted: false,
              record: { id: 'todo00000000500', title: 'Last task' },
            },
          ],
        });
      return response({ cursor: 501, changes: [] });
    });
    await startOffline('offline-test-user');
    await vi.waitFor(() => expect(page).toBeGreaterThanOrEqual(3), { timeout: 5000 });
    expect(await offlineCollection('todos').getFullList()).toHaveLength(501);
  });
  it('replays deferred remote changes after acknowledging a lost push response', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    const base = { id: 'todo00000000003', title: 'Original', description: 'Old' };
    const journal = [
      { collection: 'todos', id: base.id, revision: 1, deleted: false, record: base },
    ];
    let receipt: Record<string, unknown> | undefined;
    const sent: unknown[] = [];
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (init?.method !== 'POST') {
        const cursor = Number(new URL(String(path), 'http://test').searchParams.get('cursor'));
        return response({
          cursor: journal.length,
          changes: journal.filter((row) => row.revision > cursor),
        });
      }
      const op = JSON.parse(String(init.body)).operations[0];
      sent.push(op);
      if (!receipt) {
        const applied = { ...base, ...op.patch };
        journal.push({
          collection: 'todos',
          id: base.id,
          revision: 2,
          deleted: false,
          record: applied,
        });
        receipt = { operationId: op.operationId, status: 'applied', revision: 2, record: applied };
        journal.push({
          collection: 'todos',
          id: base.id,
          revision: 3,
          deleted: false,
          record: { ...applied, description: 'Changed remotely' },
        });
        throw new Error('Lost reply');
      }
      return response({ results: [receipt], cursor: 3 });
    });
    await startOffline('offline-test-user');
    const todos = offlineCollection('todos');
    await vi.waitFor(async () => expect(await todos.getOne(base.id)).toEqual(base));
    await todos.update(base.id, { title: 'Local' });
    const { getOfflineStatus } = await import('./index');
    await vi.waitFor(() => expect(getOfflineStatus().error).toBe('Lost reply'));
    retryOfflineSync();
    await vi.waitFor(async () =>
      expect(await todos.getOne(base.id)).toMatchObject({
        title: 'Local',
        description: 'Changed remotely',
      }),
    );
    expect(sent).toHaveLength(2);
    expect(sent[1]).toEqual(sent[0]);
  });

  it('choosing the server version drops later queued edits to that conflicted record', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    const base = {
      id: 'todo00000000004',
      title: 'Base',
      description: '',
      owner: 'offline-test-user',
    };
    let resolvePush!: (value: Response) => void;
    let sent: { operationId: string } | undefined;
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (String(path).includes('/pull?')) {
        const cursor = Number(new URL(String(path), 'http://test').searchParams.get('cursor'));
        return response({
          cursor: 1,
          changes:
            cursor < 1
              ? [{ collection: 'todos', id: base.id, revision: 1, deleted: false, record: base }]
              : [],
        });
      }
      sent = JSON.parse(String(init?.body)).operations[0];
      return new Promise<Response>((resolve) => {
        resolvePush = resolve;
      });
    });
    await startOffline('offline-test-user');
    await vi.waitFor(async () =>
      expect(await offlineCollection('todos').getOne(base.id)).toEqual(base),
    );
    const todos = offlineCollection('todos');
    await todos.update(base.id, { title: 'My title' });
    await vi.waitFor(() => expect(sent).toBeDefined());
    await todos.update(base.id, { description: 'My later description' });
    const remote = { ...base, title: 'Server title', description: 'Server description' };
    resolvePush(
      response({
        results: [
          {
            operationId: sent!.operationId,
            status: 'conflict',
            revision: 2,
            record: remote,
            message: 'same field changed',
          },
        ],
      }),
    );
    await vi.waitFor(async () => expect((await getOfflineConflicts()).length).toBe(1));
    const conflict = (await getOfflineConflicts())[0];
    expect(conflict.patch).toMatchObject({
      title: 'My title',
      description: 'My later description',
    });
    await resolveConflict(conflict.operationId, 'remote');
    expect(await todos.getOne(base.id)).toMatchObject({
      title: 'Server title',
      description: 'Server description',
    });
    expect(await getOfflineConflicts()).toEqual([]);
    expect((await todos.getFullList()).length).toBe(1);
  });

  it('waits for a locally created record to receive its canonical server copy', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (init?.method === 'POST') {
        const op = JSON.parse(String(init.body)).operations[0];
        return response({
          results: [
            {
              operationId: op.operationId,
              status: 'applied',
              revision: 1,
              record: { ...op.patch, owner: 'offline-test-user' },
            },
          ],
        });
      }
      return response({ cursor: 0, changes: [] });
    });
    await startOffline('offline-test-user');
    const local = await offlineCollection('todos').create({
      title: 'Create before reminder',
      owner: 'offline-test-user',
    });
    const canonical = await waitForOfflineRecord('todos', local.id, 1000);
    expect(canonical).toMatchObject({
      id: local.id,
      title: 'Create before reminder',
      owner: 'offline-test-user',
    });
  });

  it('keeps an acknowledged recurrence edit in the cached range for an offline reload', async () => {
    vi.stubGlobal('navigator', { onLine: true });
    auth.token = 'token';
    const master = {
      id: 'event0000000001',
      owner: 'offline-test-user',
      title: 'Weekly',
      description: 'Base',
      rrule: 'FREQ=WEEKLY',
      start_date: '2026-11-10 10:00:00.000Z',
      end_date: '2026-11-10 11:00:00.000Z',
      calendar: 'calendar0000001',
      timezone: 'UTC',
      time_mode: 'zoned',
      all_day: false,
    };
    const occurrencePath =
      '/api/calendar/occurrences?start=2026-11-01T00%3A00%3A00.000Z&end=2026-12-01T00%3A00%3A00.000Z&timezone=UTC';
    const occurrence = {
      id: `${master.id}:2026-11-10T10:00:00Z`,
      masterId: master.id,
      recurrenceId: '2026-11-10T10:00:00Z',
      start: '2026-11-10T10:00:00Z',
      end: '2026-11-10T11:00:00Z',
      startLocal: '20261110T100000',
      endLocal: '20261110T110000',
      timezone: 'UTC',
      timeMode: 'zoned',
      allDay: false,
      recurring: true,
      title: master.title,
      description: master.description,
      calendar: master.calendar,
      color: '#123456',
    };
    let initialPull = true;
    let pushes = 0;
    let holdOccurrence = false;
    let occurrenceStarted = false;
    let releaseOccurrence!: (value: Response) => void;
    vi.mocked(apiFetch).mockImplementation(async (path, init) => {
      if (init?.method === 'POST') {
        const op = JSON.parse(String(init.body)).operations[0];
        pushes++;
        const override = {
          id: 'override0000001',
          owner: master.owner,
          recurrence_parent: master.id,
          recurrence_id: occurrence.recurrenceId,
          title: 'Moved',
          description: 'Changed',
          start_date: '2026-11-10 12:00:00.000Z',
          end_date: '2026-11-10 13:00:00.000Z',
          start_local: '20261110T120000',
          end_local: '20261110T130000',
          timezone: 'UTC',
          time_mode: 'zoned',
          all_day: false,
          calendar: master.calendar,
          recurrence_cancelled: op.kind === 'cancel',
        };
        return response({
          results: [
            {
              operationId: op.operationId,
              status: 'applied',
              revision: 3 + pushes,
              record: override,
              masterRevision: 3 + pushes,
              masterRecord: { ...master, updated: '2026-11-10 12:00:00.000Z' },
            },
          ],
        });
      }
      if (String(path).startsWith('/api/calendar/occurrences?')) {
        if (holdOccurrence) {
          occurrenceStarted = true;
          return new Promise<Response>((resolve) => {
            releaseOccurrence = resolve;
          });
        }
        return response([occurrence]);
      }
      if (String(path).includes('/pull?')) {
        if (initialPull) {
          initialPull = false;
          return response({
            cursor: 1,
            changes: [
              { collection: 'events', id: master.id, revision: 1, deleted: false, record: master },
            ],
          });
        }
        return response({ cursor: 1, changes: [] });
      }
      return response({});
    });
    await startOffline('offline-test-user');
    await vi.waitFor(async () =>
      expect(await offlineCollection('events').getOne(master.id)).toEqual(master),
    );
    const first = await offlineJson<(typeof occurrence)[]>(occurrencePath);
    expect(first.data[0].title).toBe('Weekly');
    holdOccurrence = true;
    const staleRead = offlineJson<(typeof occurrence)[]>(occurrencePath);
    await vi.waitFor(() => expect(occurrenceStarted).toBe(true));
    await offlineRecurrence(master.id, 'override', occurrence.recurrenceId, {
      title: 'Moved',
      description: 'Changed',
      start_date: '2026-11-10T12:00:00Z',
      end_date: '2026-11-10T13:00:00Z',
    });
    await vi.waitFor(async () => {
      expect(pushes).toBe(1);
      expect(await getPendingRecurrenceActions()).toHaveLength(0);
    });
    releaseOccurrence(response([occurrence]));
    const staleResult = await staleRead;
    expect(staleResult.data[0]).toMatchObject({ title: 'Moved', description: 'Changed' });
    vi.stubGlobal('navigator', { onLine: false });
    const afterAck = await offlineJson<(typeof occurrence)[]>(occurrencePath);
    expect(afterAck.data[0]).toMatchObject({ title: 'Moved', description: 'Changed' });
    vi.stubGlobal('navigator', { onLine: true });
    await offlineRecurrence(master.id, 'cancel', occurrence.recurrenceId);
    await vi.waitFor(async () => {
      expect(pushes).toBe(2);
      expect(await getPendingRecurrenceActions()).toHaveLength(0);
    });
    vi.stubGlobal('navigator', { onLine: false });
    const reloaded = await offlineJson<(typeof occurrence)[]>(occurrencePath);
    expect(reloaded.cached).toBe(true);
    expect(reloaded.data).toEqual([]);
  });
  it('does not reopen an account after logout while storage is opening', async () => {
    const opening = startOffline('offline-test-user');
    auth.record = null;
    await expect(opening).rejects.toThrow('Offline account does not match');
    const { getOfflineConflicts } = await import('./index');
    expect(await getOfflineConflicts()).toEqual([]);
  });
});
