import { CalendarDays, Check, MoreHorizontal, Trash2 } from 'lucide-react';
import { apiFetch } from '../api/client';
import { pb } from '../api/pocketbase';
import type { Entry } from '../data';
import { useState } from 'react';

export function EntryRow({
  entry,
  onEdit,
  onChanged,
}: {
  entry: Entry;
  onEdit?: (entry: Entry) => void;
  onChanged?: () => void;
}) {
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const collection = entry.type === 'task' ? 'todos' : 'events';
  const toggleComplete = async () => {
    if (entry.type !== 'task' || busy) return;
    const completed = !entry.done;
    setBusy(true);
    setError('');
    try {
      await pb.collection(collection).update(entry.masterId ?? entry.id, {
        completed,
        status: completed ? 'COMPLETED' : 'NEEDS-ACTION',
        completed_at: completed ? new Date().toISOString() : '',
      });
      onChanged?.();
    } catch {
      setError('Could not update this entry.');
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (busy) return;
    const occurrence = Boolean(entry.masterId);
    if (
      !window.confirm(
        occurrence
          ? `Delete only this occurrence of “${entry.title}”?`
          : `Delete “${entry.title}”?`,
      )
    )
      return;
    setBusy(true);
    setError('');
    try {
      if (occurrence) {
        const recurrenceId = String(entry.fields?.recurrence_id);
        const response = await apiFetch(`/api/calendar/occurrences/${entry.masterId}/cancel`, {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${pb.authStore.token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ recurrenceId }),
        });
        if (!response.ok) throw new Error('Could not delete this occurrence.');
      } else {
        await pb.collection(collection).delete(entry.id);
      }
      onChanged?.();
    } catch {
      setError(occurrence ? 'Could not delete this occurrence.' : 'Could not delete this entry.');
    } finally {
      setBusy(false);
    }
  };
  return (
    <article className="entry-row">
      <div className="entry-time">{entry.time ?? 'All day'}</div>
      {error && (
        <p className="entry-error" role="alert">
          {error}
        </p>
      )}
      <button
        className="entry-icon"
        style={{ color: entry.color }}
        onClick={() => void toggleComplete()}
        disabled={entry.type !== 'task' || busy}
        aria-label={
          entry.type === 'task' ? `${entry.done ? 'Reopen' : 'Complete'} ${entry.title}` : undefined
        }
      >
        {entry.type === 'event' ? <CalendarDays size={18} /> : <Check size={18} />}
      </button>
      <div className="entry-copy">
        <strong>{entry.title}</strong>
        <span>{entry.description}</span>
        <small>
          {Object.entries(entry.fields ?? {})
            .filter(
              ([key, value]) =>
                value &&
                ![
                  'id',
                  'owner',
                  'collectionId',
                  'collectionName',
                  'created',
                  'updated',
                  'title',
                  'description',
                  'start_date',
                  'end_date',
                  'due_date',
                  'all_day',
                  'completed',
                ].includes(key),
            )
            .map(([key, value]) => `${key.replaceAll('_', ' ')}: ${String(value)}`)
            .join(' · ')}
        </small>
      </div>
      <span className="entry-list">
        <i style={{ background: entry.color }} />
        {entry.list}
      </span>
      <button
        className="icon-button row-more"
        aria-label={`Edit ${entry.title}`}
        onClick={() => onEdit?.(entry)}
      >
        <MoreHorizontal size={18} />
      </button>
      <button
        className="icon-button row-delete"
        aria-label={`Delete ${entry.title}`}
        onClick={() => void remove()}
      >
        <Trash2 size={17} />
      </button>
    </article>
  );
}
