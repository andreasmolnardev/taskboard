import { CalendarDays, Check, MoreHorizontal } from 'lucide-react';
import type { Entry } from '../data';

export function EntryRow({ entry }: { entry: Entry }) {
  return (
    <article className="entry-row">
      <div className="entry-time">{entry.time ?? 'All day'}</div>
      <div className="entry-icon" style={{ color: entry.color }}>
        {entry.type === 'event' ? <CalendarDays size={18} /> : <Check size={18} />}
      </div>
      <div className="entry-copy">
        <strong>{entry.title}</strong>
        <span>{entry.description}</span>
      </div>
      <span className="entry-list">
        <i style={{ background: entry.color }} />
        {entry.list}
      </span>
      <button className="icon-button row-more" aria-label={`More options for ${entry.title}`}>
        <MoreHorizontal size={18} />
      </button>
    </article>
  );
}
