import { pb } from '../api/pocketbase';
import { apiBaseUrl, apiFetch } from '../api/client';

export type OfflineCollectionName = 'todos' | 'events' | 'lists' | 'calendars';
export const OFFLINE_ENABLED = import.meta.env.VITE_OFFLINE_ENABLED !== 'false';
export type SyncStatus = {
  online: boolean;
  syncing: boolean;
  paused: boolean;
  pending: number;
  conflicts: number;
  lastSyncedAt: number | null;
  error: string | null;
};
export type OfflineRecord = Record<string, unknown> & { id: string; collection?: string };
export type SyncOperation = {
  operationId: string;
  collection: string;
  id: string;
  kind: 'create' | 'update' | 'delete' | 'override' | 'cancel';
  baseRevision: number;
  base: OfflineRecord | null;
  patch: Record<string, unknown>;
};
type StoredRecord = {
  collection: string;
  id: string;
  record: OfflineRecord;
  revision: number;
  deleted: boolean;
};
type StoredOperation = SyncOperation & { seq: number; inFlight?: boolean; attempted?: boolean };
type Conflict = {
  op: StoredOperation;
  remote: OfflineRecord | null;
  revision: number;
  deleted: boolean;
  reason: string;
};
export type OfflineConflict = {
  operationId: string;
  collection: string;
  id: string;
  kind: SyncOperation['kind'];
  base: OfflineRecord | null;
  patch: Record<string, unknown>;
  remote: OfflineRecord | null;
  revision: number;
  deleted: boolean;
  message: string;
};
type PullChange = {
  collection: string;
  id: string;
  revision: number;
  deleted: boolean;
  record: OfflineRecord | null;
};
type CachedJson = { data: unknown; cachedAt: number };

const supported = new Set<OfflineCollectionName>(['todos', 'events', 'lists', 'calendars']);
const groupFields = [
  [
    'start_date',
    'start_local',
    'end_date',
    'end_local',
    'timezone',
    'time_mode',
    'duration',
    'all_day',
  ],
  [
    'completed',
    'status',
    'completed_at',
    'percent_complete',
    'due_date',
    'due_local',
    'start_date',
    'start_local',
  ],
  ['rrule', 'exdate', 'recurrence_parent', 'recurrence_id', 'recurrence_cancelled'],
  ['list', 'calendar'],
];
const dbPrefix = 'taskboard-offline-v1';
let activeDb: IDBDatabase | null = null;
let activeUser = '';
let activeDbName = '';
let broadcast: BroadcastChannel | null = null;
let sessionGeneration = 0;
let workerPromise: Promise<void> | null = null;
let stopping = false;
let retryTimer: number | undefined;
let focusTimer: number | undefined;
let realtimeUnsubscribers: Array<() => void> = [];
const inflightIds = new Set<string>();
let backoffMs = 1000;
const listeners = new Set<(status: SyncStatus) => void>();
const dataListeners = new Set<() => void>();
const status: SyncStatus = {
  online: navigator.onLine,
  syncing: false,
  paused: false,
  pending: 0,
  conflicts: 0,
  lastSyncedAt: null,
  error: null,
};

function dbName(userId: string) {
  return `${dbPrefix}-${encodeURIComponent(apiBaseUrl)}-${encodeURIComponent(userId)}`;
}
function request<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('IndexedDB request failed'));
  });
}
function transactionDone(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onabort = tx.onerror = () => reject(tx.error ?? new Error('IndexedDB transaction failed'));
  });
}
function openDb(name: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(name, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains('records'))
        db.createObjectStore('records', { keyPath: ['collection', 'id'] });
      if (!db.objectStoreNames.contains('base'))
        db.createObjectStore('base', { keyPath: ['collection', 'id'] });
      if (!db.objectStoreNames.contains('outbox')) {
        const store = db.createObjectStore('outbox', { keyPath: 'operationId' });
        store.createIndex('seq', 'seq', { unique: true });
      }
      if (!db.objectStoreNames.contains('conflicts'))
        db.createObjectStore('conflicts', { keyPath: 'op.operationId' });
      if (!db.objectStoreNames.contains('meta')) db.createObjectStore('meta');
      if (!db.objectStoreNames.contains('json')) db.createObjectStore('json');
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('Could not open offline storage'));
    req.onblocked = () => reject(new Error('Offline storage upgrade is blocked by another tab'));
  });
}
function db(): IDBDatabase {
  if (!activeDb) throw new Error('Offline storage is not started');
  return activeDb;
}
function emit() {
  void refreshStatus();
  for (const listener of listeners) listener({ ...status });
}
function emitData() {
  broadcast?.postMessage({ type: 'data' });
  for (const listener of dataListeners) listener();
}
async function refreshStatus() {
  if (!activeDb) return;
  try {
    const database = activeDb;
    const generation = sessionGeneration;
    const tx = database.transaction(['outbox', 'conflicts', 'meta'], 'readonly');
    const [pending, conflicts, lastSyncedAt] = await Promise.all([
      request(tx.objectStore('outbox').count()),
      request(tx.objectStore('conflicts').count()),
      request(tx.objectStore('meta').get('lastSyncedAt')),
    ]);
    if (database !== activeDb || generation !== sessionGeneration) return;
    status.pending = pending;
    status.conflicts = conflicts;
    status.lastSyncedAt = (lastSyncedAt as number | undefined) ?? null;
    for (const listener of listeners) listener({ ...status });
  } catch {
    /* status reads must not break local edits */
  }
}
function randomId() {
  const bytes = crypto.getRandomValues(new Uint8Array(15));
  return Array.from(bytes, (byte) => 'abcdefghijklmnopqrstuvwxyz0123456789'[byte % 36]).join('');
}
function operationId() {
  return crypto.randomUUID?.() ?? `${Date.now().toString(36)}-${randomId()}`;
}
function clone<T>(value: T): T {
  return structuredClone(value);
}
function equal(a: unknown, b: unknown) {
  return JSON.stringify(a) === JSON.stringify(b);
}
function groupsFor(keys: string[]) {
  const groups: string[][] = [];
  const handled = new Set<string>();
  for (const key of keys) {
    if (handled.has(key)) continue;
    const group = groupFields.find((fields) => fields.includes(key)) ?? [key];
    group.forEach((field) => handled.add(field));
    groups.push(group);
  }
  return groups;
}
export function mergeRemoteWithLocal(
  base: OfflineRecord | null,
  local: OfflineRecord,
  remote: OfflineRecord,
): { merged: OfflineRecord; conflicts: string[] } {
  const merged = clone(remote);
  const conflicts: string[] = [];
  const keys = new Set([...Object.keys(base ?? {}), ...Object.keys(local)]);
  for (const fields of groupsFor([...keys])) {
    const localChanged = fields.some((field) => !equal(local[field], base?.[field]));
    if (!localChanged) continue;
    const remoteChanged = fields.some((field) => !equal(remote[field], base?.[field]));
    const same = fields.every((field) => equal(local[field], remote[field]));
    if (remoteChanged && !same) {
      conflicts.push(fields.join(','));
      continue;
    }
    for (const field of fields) {
      if (field in local) merged[field] = clone(local[field]);
      else delete merged[field];
    }
  }
  return { merged, conflicts };
}
function collect<T>(store: IDBObjectStore | IDBIndex): Promise<T[]> {
  return new Promise((resolve, reject) => {
    const req = store.getAll();
    req.onsuccess = () => resolve(req.result as T[]);
    req.onerror = () => reject(req.error);
  });
}
function eventToPull(change: PullChange): StoredRecord {
  return {
    collection: change.collection,
    id: change.id,
    record: change.record ?? ({ id: change.id } as OfflineRecord),
    revision: change.revision,
    deleted: change.deleted,
  };
}

async function pull() {
  let changed = false;
  const database = db();
  const generation = sessionGeneration;
  const user = activeUser;
  const tx = db().transaction('meta', 'readonly');
  const cursor = Number((await request(tx.objectStore('meta').get('cursor'))) ?? 0);
  const response = await apiFetch(`/api/sync/pull?cursor=${encodeURIComponent(cursor)}`, {
    headers: { Authorization: `Bearer ${pb.authStore.token}` },
  });
  if (database !== activeDb || generation !== sessionGeneration || user !== activeUser) return;
  if (response.status === 401 || response.status === 403) {
    status.paused = true;
    throw new Error('Sign in to resume sync');
  }
  if (!response.ok) throw new Error(`Pull failed (${response.status})`);
  const payload = (await response.json()) as { cursor: number; changes: PullChange[] };
  if (database !== activeDb || generation !== sessionGeneration || user !== activeUser) return;
  const write = db().transaction(['records', 'base', 'outbox', 'conflicts', 'meta'], 'readwrite');
  const records = write.objectStore('records');
  const bases = write.objectStore('base');
  const outbox = write.objectStore('outbox');
  const conflicts = write.objectStore('conflicts');
  const queue = await collect<StoredOperation>(outbox);
  const existingConflicts = await collect<Conflict>(conflicts);
  let deferredCursor: number | undefined;
  // Only the newest row per record in this page is relevant to a local rebase.
  const latestChanges = new Map<string, PullChange>();
  for (const change of payload.changes ?? [])
    latestChanges.set(`${change.collection}\0${change.id}`, change);
  for (const change of latestChanges.values()) {
    const remote = eventToPull(change);
    const local = await request<StoredRecord | undefined>(
      records.get([change.collection, change.id]),
    );
    const base = await request<StoredRecord | undefined>(bases.get([change.collection, change.id]));
    const pending = queue.filter(
      (op) => op.collection === change.collection && op.id === change.id,
    );
    if (base && remote.revision <= base.revision) continue;
    // A sent operation may already have a server receipt even if the response was lost.
    // Retry its exact payload before interpreting a newer pull as a conflict.
    const blocked = existingConflicts.filter(
      ({ op }) => op.collection === change.collection && op.id === change.id,
    );
    if (blocked.length) {
      for (const conflict of blocked) {
        conflicts.put({
          ...conflict,
          remote: change.record,
          revision: change.revision,
          deleted: change.deleted,
        });
      }
      bases.put(remote);
      continue;
    }
    if (pending.some((op) => op.attempted)) {
      deferredCursor = Math.min(deferredCursor ?? change.revision - 1, change.revision - 1);
      continue;
    }
    const latestBaseRevision = pending.reduce((latest, op) => Math.max(latest, op.baseRevision), 0);
    if (pending.length && remote.revision <= latestBaseRevision) continue;
    if (!pending.length) {
      if (remote.deleted) records.delete([change.collection, change.id]);
      else records.put(remote);
      bases.put(remote);
      changed = true;
      continue;
    }
    if (remote.deleted) {
      const op = pending[0];
      conflicts.put({
        op,
        remote: null,
        revision: remote.revision,
        deleted: true,
        reason: 'Record was deleted on the server',
      } satisfies Conflict);
      bases.put(remote);
      continue;
    }
    if (!local || local.deleted) {
      const op = pending[0];
      conflicts.put({
        op,
        remote: remote.record,
        revision: remote.revision,
        deleted: false,
        reason: 'The server changed a record with a pending local delete or create',
      } satisfies Conflict);
      bases.put(remote);
      continue;
    }
    const recurrenceOp = pending.find((op) => op.kind === 'override' || op.kind === 'cancel');
    if (recurrenceOp) {
      conflicts.put({
        op: recurrenceOp,
        remote: remote.record,
        revision: remote.revision,
        deleted: false,
        reason: 'The recurring event changed on the server',
      } satisfies Conflict);
      records.put(remote);
      bases.put(remote);
      continue;
    }
    const merged = mergeRemoteWithLocal(
      base?.record ?? pending[0].base,
      local.record,
      remote.record,
    );
    if (merged.conflicts.length) {
      const op = pending[0];
      conflicts.put({
        op,
        remote: remote.record,
        revision: remote.revision,
        deleted: false,
        reason: `Changed fields overlap: ${merged.conflicts.join('; ')}`,
      } satisfies Conflict);
      bases.put(remote);
      continue;
    }
    records.put({ ...remote, record: merged.merged });
    bases.put(remote);
    changed = true;
    for (const op of pending) {
      if (op.inFlight) continue;
      op.base = clone(remote.record);
      op.baseRevision = remote.revision;
      op.patch = diff(remote.record, merged.merged);
      outbox.put(op);
    }
  }
  write.objectStore('meta').put(deferredCursor ?? payload.cursor, 'cursor');
  write.objectStore('meta').put(Date.now(), 'lastSyncedAt');
  await transactionDone(write);
  if (changed) emitData();
  if (deferredCursor === undefined && (payload.changes ?? []).length === 500) await pull();
}
function diff(base: OfflineRecord, next: OfflineRecord) {
  const patch: Record<string, unknown> = {};
  for (const key of new Set([...Object.keys(base), ...Object.keys(next)]))
    if (!equal(base[key], next[key])) patch[key] = key in next ? clone(next[key]) : null;
  return patch;
}
async function push(): Promise<boolean> {
  const database = db();
  const generation = sessionGeneration;
  const user = activeUser;
  const read = db().transaction('outbox', 'readonly');
  const operations = (await collect<StoredOperation>(read.objectStore('outbox'))).sort(
    (a, b) => a.seq - b.seq,
  );
  const blocked = new Set(
    (
      await collect<Conflict>(db().transaction('conflicts', 'readonly').objectStore('conflicts'))
    ).map(({ op }) => op.operationId),
  );
  const firstByRecord = new Map<string, StoredOperation>();
  for (const op of operations) {
    const key = `${op.collection}\0${op.id}`;
    if (!firstByRecord.has(key)) firstByRecord.set(key, op);
  }
  const blockedRecords = new Set(
    operations
      .filter((op) => blocked.has(op.operationId))
      .map((op) => `${op.collection}\0${op.id}`),
  );
  const allConflicts = await collect<Conflict>(
    db().transaction('conflicts', 'readonly').objectStore('conflicts'),
  );
  const waitingParents = new Set([
    ...operations
      .filter(
        (op) =>
          op.kind === 'create' && (op.collection === 'lists' || op.collection === 'calendars'),
      )
      .map((op) => op.id),
    ...allConflicts
      .filter(
        ({ op }) =>
          op.kind === 'create' && (op.collection === 'lists' || op.collection === 'calendars'),
      )
      .map(({ op }) => op.id),
  ]);
  const dependsOnWaitingParent = (op: StoredOperation) =>
    ['list', 'calendar'].some(
      (field) =>
        typeof op.patch[field] === 'string' && waitingParents.has(op.patch[field] as string),
    );
  const eligible = [...firstByRecord.values()]
    .filter(
      (op) =>
        !op.inFlight &&
        !inflightIds.has(op.operationId) &&
        !blockedRecords.has(`${op.collection}\0${op.id}`) &&
        !dependsOnWaitingParent(op),
    )
    .slice(0, 100);
  if (!eligible.length) return false;
  const tx = db().transaction('outbox', 'readwrite');
  for (const op of eligible) {
    op.inFlight = true;
    op.attempted = true;
    tx.objectStore('outbox').put(op);
    inflightIds.add(op.operationId);
  }
  await transactionDone(tx);
  try {
    const response = await apiFetch('/api/sync/push', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${pb.authStore.token}`,
      },
      body: JSON.stringify({
        operations: eligible.map(
          ({ operationId: id, collection, id: recordId, kind, baseRevision, base, patch }) => ({
            operationId: id,
            collection,
            id: recordId,
            kind,
            baseRevision,
            base,
            patch,
          }),
        ),
      }),
    });
    if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
      return false;
    if (response.status === 401 || response.status === 403) {
      status.paused = true;
      throw new Error('Sign in to resume sync');
    }
    if (!response.ok) throw new Error(`Push failed (${response.status})`);
    const payload = (await response.json()) as {
      results: Array<{
        operationId: string;
        status: 'applied' | 'conflict' | 'error';
        revision?: number;
        record?: OfflineRecord | null;
        deleted?: boolean;
        message?: string;
        masterRevision?: number;
        masterRecord?: OfflineRecord | null;
      }>;
      cursor?: number;
    };
    if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
      return false;
    const write = database.transaction(
      ['records', 'base', 'outbox', 'conflicts', 'meta', 'json'],
      'readwrite',
    );
    const received = new Set((payload.results ?? []).map((result) => result.operationId));
    for (const result of payload.results ?? []) {
      const op = eligible.find((item) => item.operationId === result.operationId)!;
      const queued = await request<StoredOperation | undefined>(
        write.objectStore('outbox').get(op.operationId),
      );
      if (result.status === 'applied') {
        if (!queued) continue;
        const canonical = result.record ?? null;
        if (canonical) {
          const isRecurrence = op.kind === 'override' || op.kind === 'cancel';
          if (isRecurrence) {
            const overrideRow: StoredRecord = {
              collection: 'events',
              id: canonical.id,
              record: canonical,
              revision: result.revision ?? op.baseRevision + 1,
              deleted: false,
            };
            write.objectStore('records').put(overrideRow);
            write.objectStore('base').put(overrideRow);
          }
          if (isRecurrence && result.masterRecord && result.masterRevision != null) {
            const masterRevision = result.masterRevision;
            write.objectStore('base').put({
              collection: 'events',
              id: op.id,
              record: result.masterRecord,
              revision: masterRevision,
              deleted: false,
            } satisfies StoredRecord);
            const queuedForMaster = await collect<StoredOperation>(write.objectStore('outbox'));
            for (const nextOp of queuedForMaster) {
              if (
                nextOp.operationId === op.operationId ||
                nextOp.collection !== 'events' ||
                nextOp.id !== op.id ||
                nextOp.attempted
              )
                continue;
              nextOp.base = clone(result.masterRecord);
              nextOp.baseRevision = masterRevision;
              if (nextOp.kind === 'update') {
                const localMaster = await request<StoredRecord | undefined>(
                  write.objectStore('records').get(['events', op.id]),
                );
                if (localMaster && !localMaster.deleted)
                  nextOp.patch = diff(result.masterRecord, localMaster.record);
              }
              write.objectStore('outbox').put(nextOp);
            }
            await projectAcknowledgedRecurrence(write.objectStore('json'), op, canonical);
          }
          if (!isRecurrence)
            write.objectStore('base').put({
              collection: op.collection,
              id: op.id,
              record: canonical,
              revision: result.revision ?? op.baseRevision + 1,
              deleted: false,
            } satisfies StoredRecord);
          const current = await request<StoredRecord | undefined>(
            write.objectStore('records').get([op.collection, op.id]),
          );
          if (!isRecurrence && current && !current.deleted) {
            // If a newer edit was queued while this request was in flight, keep it on top of the canonical response.
            const nextOp = await firstPending(
              write.objectStore('outbox'),
              op.collection,
              op.id,
              op.operationId,
            );
            if (nextOp) {
              current.record = applyPatch(canonical, nextOp.patch);
              nextOp.base = clone(canonical);
              nextOp.baseRevision = result.revision ?? op.baseRevision + 1;
              nextOp.patch = diff(canonical, current.record);
              write.objectStore('outbox').put(nextOp);
            } else current.record = canonical;
            current.revision = result.revision ?? current.revision;
            write.objectStore('records').put(current);
          } else if (!isRecurrence) {
            const nextOp = await firstPending(
              write.objectStore('outbox'),
              op.collection,
              op.id,
              op.operationId,
            );
            if (nextOp) {
              nextOp.base = clone(canonical);
              nextOp.baseRevision = result.revision ?? op.baseRevision + 1;
              write.objectStore('outbox').put(nextOp);
            }
          }
        } else if (result.deleted ?? op.kind === 'delete')
          write.objectStore('records').delete([op.collection, op.id]);
        write.objectStore('outbox').delete(op.operationId);
        write.objectStore('conflicts').delete(op.operationId);
      } else {
        if (queued) {
          queued.inFlight = false;
          write.objectStore('outbox').put(queued);
        }
        if (result.status === 'conflict')
          write.objectStore('conflicts').put({
            op,
            remote: result.record ?? null,
            revision: result.revision ?? op.baseRevision,
            deleted: result.deleted ?? false,
            reason: result.message ?? 'Server rejected overlapping changes',
          } satisfies Conflict);
        else {
          write.objectStore('conflicts').put({
            op,
            remote: result.record ?? null,
            revision: result.revision ?? op.baseRevision,
            deleted: result.deleted ?? false,
            reason: result.message ?? 'Server rejected this change',
          } satisfies Conflict);
        }
      }
    }
    for (const op of eligible) {
      if (received.has(op.operationId)) continue;
      const queued = await request<StoredOperation | undefined>(
        write.objectStore('outbox').get(op.operationId),
      );
      if (queued) {
        queued.inFlight = false;
        write.objectStore('outbox').put(queued);
      }
    }
    write.objectStore('meta').put(Date.now(), 'lastSyncedAt');
    await transactionDone(write);
    emitData();
    return eligible.length === 100;
  } catch (error) {
    if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
      throw error;
    const tx = db().transaction('outbox', 'readwrite');
    for (const op of eligible) {
      const queued = await request<StoredOperation | undefined>(
        tx.objectStore('outbox').get(op.operationId),
      );
      if (queued) {
        queued.inFlight = false;
        tx.objectStore('outbox').put(queued);
      }
    }
    await transactionDone(tx);
    throw error;
  } finally {
    for (const op of eligible) inflightIds.delete(op.operationId);
  }
}
function applyPatch(record: OfflineRecord, patch: Record<string, unknown>) {
  const next = clone(record);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete next[key];
    else next[key] = clone(value);
  }
  return next;
}
async function firstPending(
  store: IDBObjectStore,
  collection: string,
  id: string,
  excluding: string,
) {
  const all = await collect<StoredOperation>(store);
  return all
    .filter((op) => op.collection === collection && op.id === id && op.operationId !== excluding)
    .sort((a, b) => a.seq - b.seq)[0];
}
async function syncLoop() {
  if (workerPromise || stopping || !activeDb || status.paused) return workerPromise ?? undefined;
  if (!navigator.onLine || !pb.authStore.token) {
    status.online = navigator.onLine;
    status.paused = !pb.authStore.token;
    emit();
    return;
  }
  const generation = sessionGeneration;
  workerPromise = (async () => {
    const run = async () => {
      if (generation !== sessionGeneration || stopping || !activeDb) return;
      status.syncing = true;
      status.error = null;
      emit();
      try {
        await pull();
        while (await push()) {
          /* drain independent operation heads */
        }
        await pull();
        if (generation === sessionGeneration) {
          backoffMs = 1000;
          status.paused = false;
          status.error = null;
        }
      } catch (error) {
        if (generation === sessionGeneration) {
          status.error = error instanceof Error ? error.message : String(error);
          if (!status.paused) scheduleRetry();
        }
      } finally {
        if (generation === sessionGeneration) {
          status.syncing = false;
          emit();
        }
      }
    };
    if ('locks' in navigator) await navigator.locks.request(activeDbName, async () => run());
    else await run();
  })().finally(() => {
    workerPromise = null;
    if (!stopping && activeDb && generation !== sessionGeneration) {
      void syncLoop();
      return;
    }
    if (
      !stopping &&
      !status.paused &&
      !status.error &&
      activeDb &&
      navigator.onLine &&
      pb.authStore.token
    ) {
      void hasRunnableOperation().then((ready) => {
        if (ready) void syncLoop();
      });
    }
  });
  return workerPromise;
}
function scheduleRetry() {
  if (retryTimer !== undefined || stopping) return;
  const jitter = Math.random() * Math.min(backoffMs / 3, 5000);
  retryTimer = window.setTimeout(() => {
    retryTimer = undefined;
    void syncLoop();
  }, backoffMs + jitter);
  backoffMs = Math.min(backoffMs * 2, 60000);
}
async function hasRunnableOperation() {
  if (!activeDb) return false;
  const tx = activeDb.transaction(['outbox', 'conflicts'], 'readonly');
  const [operations, conflictRows] = await Promise.all([
    collect<StoredOperation>(tx.objectStore('outbox')),
    collect<Conflict>(tx.objectStore('conflicts')),
  ]);
  const blocked = new Set(conflictRows.map(({ op }) => `${op.collection}\0${op.id}`));
  const waitingParents = new Set([
    ...operations
      .filter(
        (op) =>
          op.kind === 'create' && (op.collection === 'lists' || op.collection === 'calendars'),
      )
      .map((op) => op.id),
    ...conflictRows
      .filter(
        ({ op }) =>
          op.kind === 'create' && (op.collection === 'lists' || op.collection === 'calendars'),
      )
      .map(({ op }) => op.id),
  ]);
  return operations.some(
    (op) =>
      !op.inFlight &&
      !blocked.has(`${op.collection}\0${op.id}`) &&
      !['list', 'calendar'].some(
        (field) =>
          typeof op.patch[field] === 'string' && waitingParents.has(op.patch[field] as string),
      ),
  );
}
function onOnline() {
  status.online = true;
  emit();
  void syncLoop();
}
function onOffline() {
  status.online = false;
  emit();
}

export async function startOffline(userId: string) {
  if (!OFFLINE_ENABLED) return;
  if (!userId) throw new Error('A signed-in user is required for offline storage');
  if (activeDb && activeUser === userId) {
    status.paused = false;
    void syncLoop();
    return;
  }
  await stopOffline(false);
  stopping = false;
  sessionGeneration++;
  activeUser = userId;
  activeDbName = dbName(userId);
  status.online = navigator.onLine;
  status.paused = false;
  status.error = null;
  status.lastSyncedAt = null;
  status.pending = 0;
  status.conflicts = 0;
  const openingGeneration = sessionGeneration;
  const openingName = activeDbName;
  try {
    const opened = await openDb(openingName);
    if (openingGeneration !== sessionGeneration) {
      opened.close();
      throw new Error('Account changed while opening offline storage');
    }
    activeDb = opened;
  } catch (error) {
    if (openingGeneration === sessionGeneration) activeUser = '';
    throw new Error(
      `Could not open offline storage: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const authenticatedUser = pb.authStore.record?.id;
  if (authenticatedUser !== userId) {
    await stopOffline(!authenticatedUser);
    throw new Error('Offline account does not match the signed-in account');
  }
  window.addEventListener('online', onOnline);
  window.addEventListener('offline', onOffline);
  window.addEventListener('focus', onFocus);
  const dbRef = activeDb;
  dbRef.onversionchange = () => {
    dbRef.close();
    if (activeDb === dbRef) {
      activeDb = null;
      activeUser = '';
    }
  };
  const generation = sessionGeneration;
  if ('BroadcastChannel' in window) {
    broadcast = new BroadcastChannel(`taskboard-offline-${activeDbName}`);
    broadcast.onmessage = (event) => {
      if (generation !== sessionGeneration) return;
      if (event.data?.type === 'data') {
        void refreshStatus();
        for (const listener of dataListeners) listener();
        void syncLoop();
      }
      if (event.data?.type === 'sync') void syncLoop();
      if (event.data?.type === 'clear') void stopOffline(false);
    };
  }
  const recovery = dbRef.transaction('outbox', 'readwrite');
  for (const op of await collect<StoredOperation>(recovery.objectStore('outbox')))
    if (op.inFlight) {
      op.inFlight = false;
      recovery.objectStore('outbox').put(op);
    }
  await transactionDone(recovery);
  for (const collection of supported) {
    void Promise.resolve()
      .then(() => pb.collection(collection).subscribe('*', () => void syncLoop()))
      .then(() => {
        if (generation !== sessionGeneration || activeDb !== dbRef) {
          void pb.collection(collection).unsubscribe('*');
          return;
        }
        realtimeUnsubscribers.push(() => {
          void pb.collection(collection).unsubscribe('*');
        });
      })
      .catch(() => {
        /* realtime is a hint; pull cursor remains authoritative */
      });
  }
  await refreshStatus();
  void syncLoop();
}
export async function stopOffline(clear = false) {
  stopping = true;
  const stopGeneration = ++sessionGeneration;
  window.removeEventListener('online', onOnline);
  window.removeEventListener('offline', onOffline);
  window.removeEventListener('focus', onFocus);
  if (retryTimer !== undefined) clearTimeout(retryTimer);
  if (focusTimer !== undefined) clearTimeout(focusTimer);
  retryTimer = focusTimer = undefined;
  realtimeUnsubscribers.forEach((unsubscribe) => unsubscribe());
  realtimeUnsubscribers = [];
  if (clear) broadcast?.postMessage({ type: 'clear' });
  broadcast?.close();
  broadcast = null;
  if (activeDb) {
    activeDb.close();
    activeDb = null;
  }
  if (clear && activeDbName)
    await new Promise<void>((resolve, reject) => {
      const req = indexedDB.deleteDatabase(activeDbName);
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error);
    });
  if (stopGeneration !== sessionGeneration) return;
  activeUser = '';
  activeDbName = '';
  status.syncing = false;
  status.paused = false;
  status.pending = 0;
  status.conflicts = 0;
  status.lastSyncedAt = null;
  emit();
}
export function subscribeOffline(listener: (status: SyncStatus) => void) {
  listeners.add(listener);
  listener({ ...status });
  return () => {
    listeners.delete(listener);
  };
}
export function subscribeOfflineData(listener: () => void) {
  dataListeners.add(listener);
  return () => {
    dataListeners.delete(listener);
  };
}
export function getOfflineStatus(): SyncStatus {
  return { ...status };
}
export function retryOfflineSync() {
  status.paused = false;
  void syncLoop();
}
function onFocus() {
  if (focusTimer) clearTimeout(focusTimer);
  focusTimer = window.setTimeout(() => void syncLoop(), 250);
}
export async function waitForOfflineRecord(
  collection: OfflineCollectionName,
  id: string,
  timeoutMs = 20000,
): Promise<OfflineRecord> {
  if (!OFFLINE_ENABLED)
    return (await pb.collection(collection).getOne(id)) as unknown as OfflineRecord;
  if (!activeDb) throw new Error('Offline storage is not started');
  const database = activeDb;
  const generation = sessionGeneration;
  const user = activeUser;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
      throw new Error('Account changed before the record synced');
    const tx = database.transaction(['records', 'base', 'outbox', 'conflicts'], 'readonly');
    const [local, base, queue, conflicts] = await Promise.all([
      request<StoredRecord | undefined>(tx.objectStore('records').get([collection, id])),
      request<StoredRecord | undefined>(tx.objectStore('base').get([collection, id])),
      collect<StoredOperation>(tx.objectStore('outbox')),
      collect<Conflict>(tx.objectStore('conflicts')),
    ]);
    if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
      throw new Error('Account changed before the record synced');
    const issue = conflicts.find(({ op }) => op.collection === collection && op.id === id);
    if (issue) throw new Error(issue.reason);
    if (status.paused) throw new Error('Sign in again before waiting for sync');
    if (!local || local.deleted) throw new Error('The local record was removed before it synced');
    const pending = queue.some((op) => op.collection === collection && op.id === id);
    if (!pending && base && !base.deleted) return clone(base.record);
    if (!navigator.onLine) throw new Error('Connection lost before the record synced');
    await wait(150);
  }
  throw new Error('Timed out waiting for the record to sync');
}
function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
export async function getOfflineConflicts(): Promise<OfflineConflict[]> {
  if (!activeDb) return [];
  const tx = db().transaction(['conflicts', 'records', 'outbox'], 'readonly');
  const [items, records, operations] = await Promise.all([
    collect<Conflict>(tx.objectStore('conflicts')),
    collect<StoredRecord>(tx.objectStore('records')),
    collect<StoredOperation>(tx.objectStore('outbox')),
  ]);
  return items.map(({ op, remote, revision, deleted, reason }) => {
    const isRecurrence = op.kind === 'override' || op.kind === 'cancel';
    const local = records.find(
      (row) => row.collection === op.collection && row.id === op.id && !row.deleted,
    );
    const related = operations
      .filter((item) => item.collection === op.collection && item.id === op.id)
      .sort((a, b) => a.seq - b.seq);
    const latest = related.at(-1);
    const patch = isRecurrence
      ? op.patch
      : local
        ? remote && !deleted
          ? diff(remote, local.record)
          : local.record
        : {};
    return {
      operationId: op.operationId,
      collection: op.collection,
      id: op.id,
      kind: isRecurrence ? op.kind : (latest?.kind ?? op.kind),
      base: op.base,
      patch,
      remote,
      revision,
      deleted,
      message: reason,
    };
  });
}

export function offlineCollection<T extends OfflineRecord = OfflineRecord>(
  name: OfflineCollectionName,
) {
  if (!supported.has(name)) throw new Error(`Unsupported offline collection: ${name}`);
  if (!OFFLINE_ENABLED) {
    const collection = pb.collection(name);
    return {
      getFullList: (options?: { sort?: string; filter?: string }) =>
        collection.getFullList(options) as Promise<T[]>,
      getOne: (id: string) => collection.getOne(id) as Promise<T>,
      create: (input: Omit<T, 'id'> & { id?: string }) => collection.create(input) as Promise<T>,
      update: (id: string, patch: Partial<T>) => collection.update(id, patch) as Promise<T>,
      delete: (id: string) => collection.delete(id),
    };
  }
  return {
    async getFullList(options?: { sort?: string; filter?: string }) {
      const tx = db().transaction('records', 'readonly');
      const records = await collect<StoredRecord>(tx.objectStore('records'));
      const output = records
        .filter((row) => row.collection === name && !row.deleted)
        .map((row) => clone(row.record) as T);
      const sort = options?.sort;
      if (sort) {
        const desc = sort.startsWith('-');
        const field = sort.replace(/^-/, '');
        output.sort(
          (a, b) => String(a[field] ?? '').localeCompare(String(b[field] ?? '')) * (desc ? -1 : 1),
        );
      }
      return output;
    },
    async getOne(id: string) {
      const tx = db().transaction('records', 'readonly');
      const row = await request<StoredRecord | undefined>(
        tx.objectStore('records').get([name, id]),
      );
      return row && !row.deleted ? (clone(row.record) as T) : null;
    },
    async create(input: Omit<T, 'id'> & { id?: string }) {
      const id = input.id ?? randomId();
      const record = { ...clone(input), id } as T;
      const tx = db().transaction(['records', 'base', 'outbox', 'meta'], 'readwrite');
      const seq = (Number(await request(tx.objectStore('meta').get('seq'))) || 0) + 1;
      tx.objectStore('meta').put(seq, 'seq');
      tx.objectStore('records').put({
        collection: name,
        id,
        record,
        revision: 0,
        deleted: false,
      } satisfies StoredRecord);
      tx.objectStore('outbox').put({
        operationId: operationId(),
        seq,
        collection: name,
        id,
        kind: 'create',
        baseRevision: 0,
        base: null,
        patch: record,
      } satisfies StoredOperation);
      await transactionDone(tx);
      emit();
      emitData();
      void syncLoop();
      return clone(record);
    },
    async update(id: string, patch: Partial<T>) {
      const tx = db().transaction(['records', 'base', 'outbox', 'meta'], 'readwrite');
      const records = tx.objectStore('records');
      const current = await request<StoredRecord | undefined>(records.get([name, id]));
      if (!current || current.deleted) {
        tx.abort();
        throw new Error(`No ${name} record with id ${id}`);
      }
      const baseRow = await request<StoredRecord | undefined>(
        tx.objectStore('base').get([name, id]),
      );
      const base = baseRow?.record ?? current.record;
      const updated = { ...current.record, ...clone(patch), id } as T;
      current.record = updated;
      records.put(current);
      const queue = await collect<StoredOperation>(tx.objectStore('outbox'));
      const editable = queue
        .filter((op) => op.collection === name && op.id === id && !op.inFlight && !op.attempted)
        .sort((a, b) => b.seq - a.seq)[0];
      if (editable?.kind === 'create') {
        editable.patch = updated;
        tx.objectStore('outbox').put(editable);
      } else if (editable?.kind === 'update') editable.patch = diff(base, updated);
      else {
        const seq = (Number(await request(tx.objectStore('meta').get('seq'))) || 0) + 1;
        tx.objectStore('meta').put(seq, 'seq');
        tx.objectStore('outbox').put({
          operationId: operationId(),
          seq,
          collection: name,
          id,
          kind: 'update',
          baseRevision: baseRow?.revision ?? current.revision,
          base: clone(base),
          patch: diff(base, updated),
        } satisfies StoredOperation);
      }
      if (editable && editable.kind !== 'create') tx.objectStore('outbox').put(editable);
      await transactionDone(tx);
      emit();
      emitData();
      void syncLoop();
      return clone(updated);
    },
    async delete(id: string) {
      const tx = db().transaction(['records', 'base', 'outbox', 'meta'], 'readwrite');
      const records = tx.objectStore('records');
      const current = await request<StoredRecord | undefined>(records.get([name, id]));
      if (!current || current.deleted) {
        await transactionDone(tx);
        return;
      }
      const queue = await collect<StoredOperation>(tx.objectStore('outbox'));
      const existing = queue
        .filter((op) => op.collection === name && op.id === id && !op.inFlight && !op.attempted)
        .sort((a, b) => b.seq - a.seq)[0];
      if (existing?.kind === 'create') tx.objectStore('outbox').delete(existing.operationId);
      else {
        const baseRow = await request<StoredRecord | undefined>(
          tx.objectStore('base').get([name, id]),
        );
        const seq = (Number(await request(tx.objectStore('meta').get('seq'))) || 0) + 1;
        tx.objectStore('meta').put(seq, 'seq');
        tx.objectStore('outbox').put({
          operationId: operationId(),
          seq,
          collection: name,
          id,
          kind: 'delete',
          baseRevision: baseRow?.revision ?? current.revision,
          base: baseRow?.record ?? current.record,
          patch: {},
        } satisfies StoredOperation);
      }
      records.delete([name, id]);
      await transactionDone(tx);
      emit();
      emitData();
      void syncLoop();
    },
  };
}

export async function resolveConflict(
  opId: string,
  choice: 'local' | 'remote' | { record: OfflineRecord },
) {
  const tx = db().transaction(['records', 'base', 'outbox', 'conflicts', 'meta'], 'readwrite');
  const conflict = await request<Conflict | undefined>(tx.objectStore('conflicts').get(opId));
  if (!conflict) {
    await transactionDone(tx);
    return;
  }
  const { op, remote, revision, deleted } = conflict;
  const outbox = tx.objectStore('outbox');
  const conflicts = tx.objectStore('conflicts');
  const isRecurrence = op.kind === 'override' || op.kind === 'cancel';
  const queued = await collect<StoredOperation>(outbox);
  const conflictRows = await collect<Conflict>(conflicts);
  const relatedOps = isRecurrence
    ? queued.filter((item) => item.operationId === opId)
    : queued.filter((item) => item.collection === op.collection && item.id === op.id);
  const relatedConflicts = isRecurrence
    ? conflictRows.filter((item) => item.op.operationId === opId)
    : conflictRows.filter((item) => item.op.collection === op.collection && item.op.id === op.id);
  for (const item of relatedOps) outbox.delete(item.operationId);
  for (const item of relatedConflicts) conflicts.delete(item.op.operationId);

  if (isRecurrence) {
    if (choice === 'remote') {
      if (remote && !deleted)
        tx.objectStore('records').put({
          collection: op.collection,
          id: op.id,
          record: remote,
          revision,
          deleted: false,
        } satisfies StoredRecord);
    } else {
      const replacement: StoredOperation = {
        ...op,
        operationId: operationId(),
        inFlight: false,
        attempted: false,
        baseRevision: revision,
        base: remote,
      };
      outbox.put(replacement);
      if (remote && !deleted)
        tx.objectStore('records').put({
          collection: op.collection,
          id: op.id,
          record: remote,
          revision,
          deleted: false,
        } satisfies StoredRecord);
    }
  } else if (choice === 'remote') {
    if (remote && !deleted)
      tx.objectStore('records').put({
        collection: op.collection,
        id: op.id,
        record: remote,
        revision,
        deleted: false,
      } satisfies StoredRecord);
    else tx.objectStore('records').delete([op.collection, op.id]);
  } else {
    const local =
      choice === 'local'
        ? await request<StoredRecord | undefined>(
            tx.objectStore('records').get([op.collection, op.id]),
          )
        : { collection: op.collection, id: op.id, record: choice.record, revision, deleted: false };
    const seq = (Number(await request(tx.objectStore('meta').get('seq'))) || 0) + 1;
    tx.objectStore('meta').put(seq, 'seq');
    if (local) {
      const kind = deleted || !remote ? 'create' : 'update';
      outbox.put({
        ...op,
        operationId: operationId(),
        seq,
        inFlight: false,
        attempted: false,
        baseRevision: revision,
        base: remote,
        kind,
        patch: remote && !deleted ? diff(remote, local.record) : local.record,
      } satisfies StoredOperation);
      tx.objectStore('records').put({ ...local, revision, deleted: false });
    } else if (remote && !deleted) {
      outbox.put({
        ...op,
        operationId: operationId(),
        seq,
        inFlight: false,
        attempted: false,
        baseRevision: revision,
        base: remote,
        kind: 'delete',
        patch: {},
      } satisfies StoredOperation);
      tx.objectStore('records').delete([op.collection, op.id]);
    } else {
      tx.objectStore('records').delete([op.collection, op.id]);
    }
  }
  if (remote)
    tx.objectStore('base').put({
      collection: op.collection,
      id: op.id,
      record: remote,
      revision,
      deleted,
    } satisfies StoredRecord);
  else if (deleted)
    tx.objectStore('base').put({
      collection: op.collection,
      id: op.id,
      record: { id: op.id } as OfflineRecord,
      revision,
      deleted: true,
    } satisfies StoredRecord);
  else tx.objectStore('base').delete([op.collection, op.id]);
  await transactionDone(tx);
  emit();
  emitData();
  void syncLoop();
}

export async function offlineRecurrence(
  masterId: string,
  kind: 'override' | 'cancel',
  recurrenceId: string,
  values?: Record<string, unknown>,
) {
  if (!OFFLINE_ENABLED) throw new Error('Offline recurrence edits are disabled');
  const collection = 'events';
  const tx = db().transaction(['records', 'base', 'outbox', 'meta'], 'readwrite');
  const base = await request<StoredRecord | undefined>(
    tx.objectStore('base').get([collection, masterId]),
  );
  const master = await request<StoredRecord | undefined>(
    tx.objectStore('records').get([collection, masterId]),
  );
  if (!base || !master || master.deleted) {
    tx.abort();
    throw new Error('The recurring event must be cached before editing an occurrence');
  }
  const seq = (Number(await request(tx.objectStore('meta').get('seq'))) || 0) + 1;
  tx.objectStore('meta').put(seq, 'seq');
  const patch = { recurrenceId, ...(kind === 'override' ? { values: clone(values ?? {}) } : {}) };
  const op: StoredOperation = {
    operationId: operationId(),
    seq,
    collection,
    id: masterId,
    kind,
    baseRevision: base.revision,
    base: clone(base.record),
    patch,
  };
  tx.objectStore('outbox').put(op);
  await transactionDone(tx);
  emit();
  void syncLoop();
  return op.operationId;
}

export async function getPendingRecurrenceActions(masterId?: string) {
  const tx = db().transaction('outbox', 'readonly');
  return (await collect<StoredOperation>(tx.objectStore('outbox')))
    .filter(
      (op) => (op.kind === 'override' || op.kind === 'cancel') && (!masterId || op.id === masterId),
    )
    .sort((a, b) => a.seq - b.seq)
    .map((op) => ({
      operationId: op.operationId,
      masterId: op.id,
      kind: op.kind as 'override' | 'cancel',
      recurrenceId: String(op.patch.recurrenceId),
      values: op.patch.values as Record<string, unknown> | undefined,
    }));
}

export type OfflineJsonResult<T> = { data: T; cached: boolean };
export async function offlineJson<T>(path: string): Promise<OfflineJsonResult<T>> {
  const key = path.startsWith('/') ? path : `/${path}`;
  const occurrencePath = key.startsWith('/api/calendar/occurrences?');
  const result = await offlineSnapshot(key, async () => {
    const response = await apiFetch(key, {
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    });
    if (response.status === 401 || response.status === 403)
      throw new Error(`Request failed (${response.status})`);
    if (!response.ok) throw new Error(`Request failed (${response.status})`);
    return (await response.json()) as T;
  });
  const timezone = occurrencePath
    ? (new URLSearchParams(key.slice(key.indexOf('?') + 1)).get('timezone') ?? undefined)
    : undefined;
  return occurrencePath && OFFLINE_ENABLED
    ? { ...result, data: await overlayRecurrenceActions(result.data, timezone) }
    : result;
}

export async function offlineSnapshot<T>(
  key: string,
  load: () => Promise<T>,
): Promise<OfflineJsonResult<T>> {
  if (!OFFLINE_ENABLED) return { data: await load(), cached: false };
  const storeKey = `json:${key.startsWith('/') ? key : `/${key}`}`;
  const database = activeDb;
  const generation = sessionGeneration;
  const user = activeUser;
  if (!database) throw new Error('Offline storage is not started');
  if (navigator.onLine && pb.authStore.token) {
    try {
      const data = await load();
      if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
        throw new Error('Account changed during cached request');
      const tx = database.transaction('json', 'readwrite');
      tx.objectStore('json').put({ data, cachedAt: Date.now() }, storeKey);
      await transactionDone(tx);
      return { data, cached: false };
    } catch (error) {
      if (error instanceof Error && /401|403/.test(error.message)) throw error;
    }
  }
  if (database !== activeDb || generation !== sessionGeneration || user !== activeUser)
    throw new Error('Account changed during cached request');
  const tx = database.transaction('json', 'readonly');
  const row = await request<{ data: T } | undefined>(tx.objectStore('json').get(storeKey));
  if (row) return { data: row.data, cached: true };
  throw new Error('No cached response is available for this range');
}

async function overlayRecurrenceActions<T>(data: T, timezone?: string): Promise<T> {
  if (!Array.isArray(data)) return data;
  const tx = db().transaction(['records', 'outbox'], 'readonly');
  const [eventRows, outbox] = await Promise.all([
    collect<StoredRecord>(tx.objectStore('records')),
    collect<StoredOperation>(tx.objectStore('outbox')),
  ]);
  let output = data as Array<Record<string, unknown>>;
  const confirmed = eventRows
    .filter(
      (row) => row.collection === 'events' && Boolean(row.record.recurrence_parent) && !row.deleted,
    )
    .sort((a, b) => a.revision - b.revision);
  for (const row of confirmed) {
    const record = row.record;
    output = applyRecurrenceRecord(
      output,
      String(record.recurrence_parent),
      String(record.recurrence_id),
      record,
      timezone,
      Boolean(record.recurrence_cancelled),
    );
  }
  const actions = outbox
    .filter((op) => op.kind === 'override' || op.kind === 'cancel')
    .sort((a, b) => a.seq - b.seq)
    .map((op) => ({
      id: op.id,
      kind: op.kind as 'override' | 'cancel',
      recurrenceId: String(op.patch.recurrenceId),
      values: op.patch.values as Record<string, unknown> | undefined,
    }));
  for (const action of actions) {
    output = applyRecurrenceRecord(
      output,
      action.id,
      action.recurrenceId,
      action.values ?? {},
      timezone,
      action.kind === 'cancel',
    );
  }
  return output as T;
}

function applyRecurrenceRecord(
  items: Array<Record<string, unknown>>,
  masterId: string,
  recurrenceId: string,
  record: Record<string, unknown>,
  timezone: string | undefined,
  cancelled: boolean,
) {
  const target = (item: Record<string, unknown>) =>
    item.masterId === masterId && sameRecurrenceId(item.recurrenceId, recurrenceId);
  if (cancelled) return items.filter((item) => !target(item));
  return items.map((item) => {
    if (!target(item)) return item;
    const allDay = Boolean(record.all_day ?? item.allDay);
    const timeMode = String(record.time_mode ?? item.timeMode ?? '');
    return {
      ...item,
      id: `${masterId}:${recurrenceId}`,
      masterId,
      recurrenceId,
      start:
        occurrenceDate(record.start_date, timezone, record.start_local, allDay, timeMode) ??
        record.start ??
        item.start,
      end:
        occurrenceDate(record.end_date, timezone, record.end_local, allDay, timeMode) ??
        record.end ??
        item.end,
      startLocal:
        occurrenceLocal(record.start_date, timezone, record.start_local, allDay, timeMode) ??
        record.start_local ??
        item.startLocal,
      endLocal:
        occurrenceLocal(record.end_date, timezone, record.end_local, allDay, timeMode) ??
        record.end_local ??
        item.endLocal,
      timezone: record.timezone ?? item.timezone,
      timeMode: record.time_mode ?? record.timeMode ?? item.timeMode,
      allDay,
      title: record.title ?? item.title,
      description: record.description ?? item.description,
      calendar: record.calendar ?? item.calendar,
      recurring: true,
    };
  });
}

async function projectAcknowledgedRecurrence(
  store: IDBObjectStore,
  op: StoredOperation,
  record: OfflineRecord,
) {
  const keys = await request(store.getAllKeys());
  const targetId = String(record.recurrence_id ?? op.patch.recurrenceId ?? '');
  const cancelled = op.kind === 'cancel' || Boolean(record.recurrence_cancelled);
  for (const key of keys) {
    if (typeof key !== 'string' || !key.startsWith('json:/api/calendar/occurrences?')) continue;
    const cached = await request<CachedJson | undefined>(store.get(key));
    if (!cached || !Array.isArray(cached.data)) continue;
    const params = new URLSearchParams(key.slice(key.indexOf('?') + 1));
    const timezone = params.get('timezone') ?? undefined;
    let changed = false;
    let occurrences = cached.data as Array<Record<string, unknown>>;
    if (cancelled) {
      const filtered = occurrences.filter(
        (item) => !(item.masterId === op.id && sameRecurrenceId(item.recurrenceId, targetId)),
      );
      changed = filtered.length !== occurrences.length;
      occurrences = filtered;
    } else {
      occurrences = occurrences.map((item) => {
        if (item.masterId !== op.id || !sameRecurrenceId(item.recurrenceId, targetId)) return item;
        changed = true;
        const allDay = Boolean(record.all_day);
        const timeMode = String(record.time_mode ?? item.timeMode ?? '');
        const start =
          occurrenceDate(record.start_date, timezone, record.start_local, allDay, timeMode) ??
          item.start;
        const end =
          occurrenceDate(record.end_date, timezone, record.end_local, allDay, timeMode) ?? item.end;
        return {
          ...item,
          id: `${op.id}:${targetId}`,
          recurrenceId: targetId,
          start,
          end,
          startLocal:
            occurrenceLocal(record.start_date, timezone, record.start_local, allDay, timeMode) ??
            item.startLocal,
          endLocal:
            occurrenceLocal(record.end_date, timezone, record.end_local, allDay, timeMode) ??
            item.endLocal,
          timezone: record.timezone ?? item.timezone,
          timeMode: record.time_mode ?? item.timeMode,
          allDay: record.all_day ?? item.allDay,
          title: record.title ?? item.title,
          description: record.description ?? item.description,
          calendar: record.calendar ?? item.calendar,
          recurring: true,
        };
      });
    }
    if (changed) store.put({ ...cached, data: occurrences }, key);
  }
}

function sameRecurrenceId(left: unknown, right: string) {
  const normalize = (value: unknown) => {
    const text = String(value ?? '');
    if (/^\d{8}$/.test(text))
      return Date.UTC(
        Number(text.slice(0, 4)),
        Number(text.slice(4, 6)) - 1,
        Number(text.slice(6, 8)),
      );
    const time = Date.parse(text);
    return Number.isNaN(time) ? text : time;
  };
  return normalize(left) === normalize(right);
}

function occurrenceDate(
  value: unknown,
  timezone?: string,
  localValue?: unknown,
  allDay = false,
  timeMode = '',
) {
  const date = new Date(String(value ?? ''));
  if (!Number.isFinite(date.getTime())) return undefined;
  if (allDay || timeMode === 'floating')
    return localOccurrenceDate(localValue) ?? date.toISOString();
  if (!timezone) return date.toISOString();
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(date);
  const fields = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${fields.year}-${fields.month}-${fields.day}T${fields.hour}:${fields.minute}:${fields.second}`;
}

function occurrenceLocal(
  value: unknown,
  timezone: string | undefined,
  localValue: unknown,
  allDay: boolean,
  timeMode: string,
) {
  if (allDay || timeMode === 'floating') return localOccurrenceCompact(localValue, allDay);
  const formatted = occurrenceDate(value, timezone, localValue, allDay, timeMode);
  if (!formatted) return undefined;
  const compact = formatted.replaceAll('-', '').replaceAll(':', '');
  return allDay ? compact.slice(0, 8) : `${compact.slice(0, 8)}T${compact.slice(9, 15)}`;
}

function localOccurrenceCompact(value: unknown, allDay: boolean) {
  const raw = String(value ?? '').trim();
  const dateTime = raw.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/);
  if (dateTime) return allDay ? `${dateTime[1]}${dateTime[2]}${dateTime[3]}` : raw;
  const compactDate = raw.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (compactDate) return allDay ? raw : `${raw}T000000`;
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2})(?::(\d{2}))?)?/);
  if (iso)
    return allDay
      ? `${iso[1]}${iso[2]}${iso[3]}`
      : `${iso[1]}${iso[2]}${iso[3]}T${iso[4] ?? '00'}${iso[5] ?? '00'}${iso[6] ?? '00'}`;
  return undefined;
}

function localOccurrenceDate(value: unknown) {
  const compact = localOccurrenceCompact(value, false);
  if (!compact) return undefined;
  return `${compact.slice(0, 4)}-${compact.slice(4, 6)}-${compact.slice(6, 8)}T${compact.slice(9, 11)}:${compact.slice(11, 13)}:${compact.slice(13, 15)}`;
}
