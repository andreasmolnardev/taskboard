import { useState } from 'react';
import { ChevronDown, Filter } from 'lucide-react';
import { entries, formatDate, getActiveContainers, lists, type Entry } from '../../data';
import { CalendarMonth } from '../calendar-month';
import { EntryRow } from '../entry-row';

export function UpcomingTab({
  onEdit,
  onChanged,
}: {
  onEdit: (entry: Entry) => void;
  onChanged: () => void;
}) {
  const now = new Date();
  const [filterOpen, setFilterOpen] = useState(false);
  const [filter, setFilter] = useState<'all' | 'task' | 'event'>('all');
  const visibleEntries = entries.filter((entry) => filter === 'all' || entry.type === filter);
  const grouped = visibleEntries.reduce<Record<string, Entry[]>>((result, entry) => {
    (result[entry.date] ??= []).push(entry);
    return result;
  }, {});

  return (
    <>
      <header className="page-header">
        <h1>Upcoming</h1>
        <div className="header-actions">
          <div className="filter-wrap">
            <button
              className={`button button-quiet ${filter !== 'all' ? 'selected' : ''}`}
              onClick={() => setFilterOpen(!filterOpen)}
            >
              <Filter size={16} /> Filter <ChevronDown size={15} />
            </button>
            {filterOpen && (
              <div className="popover filter-popover">
                <strong>Show only</strong>
                {(['all', 'task', 'event'] as const).map((value) => (
                  <button
                    key={value}
                    onClick={() => {
                      setFilter(value);
                      setFilterOpen(false);
                    }}
                  >
                    <span className={`radio ${filter === value ? 'checked' : ''}`} />
                    {value === 'all' ? 'Everything' : `${value[0].toUpperCase()}${value.slice(1)}s`}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </header>
      <section className="calendar-strip" aria-label="Upcoming months">
        <CalendarMonth year={now.getFullYear()} month={now.getMonth()} />
        <CalendarMonth year={now.getFullYear()} month={now.getMonth() + 1} />
      </section>
      <div className="list-legend">
        {getActiveContainers(lists).map((list) => (
          <span key={list.name}>
            <i style={{ background: list.color }} />
            {list.name}
          </span>
        ))}
      </div>
      <section className="entry-groups">
        {Object.entries(grouped).map(([date, dayEntries]) => (
          <div className="entry-group" key={date}>
            <h2>{formatDate(date)}</h2>
            {dayEntries.map((entry) => (
              <EntryRow entry={entry} key={entry.id} onEdit={onEdit} onChanged={onChanged} />
            ))}
          </div>
        ))}
      </section>
    </>
  );
}
