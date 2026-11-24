import { useState, type CSSProperties } from 'react';
import { MoreHorizontal, Square, X } from 'lucide-react';
import { entries, formatDateKey, getWeekStart, type Entry } from '../data';

function formatEntryTime(entry: Entry) {
  const rawValue = entry.type === 'task' ? entry.fields?.due_date : entry.fields?.start_date;
  const value = String(rawValue ?? '').trim();
  const time = entry.time || (value.match(/T(\d{2}:\d{2})/)?.[1] ?? '');
  if (!time) return '';

  const date = new Date(`${entry.date}T${time}:00`);
  if (Number.isNaN(date.getTime())) return time;
  return new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit' }).format(date);
}

function isAllDayEvent(entry: Entry) {
  return entry.type === 'event' && entry.fields?.all_day === true;
}

function formatEntryRange(entry: Entry) {
  const start = formatEntryTime(entry);
  if (!start || isAllDayEvent(entry)) return isAllDayEvent(entry) ? 'All day' : 'No time set';
  const endValue = String(entry.fields?.end_date ?? '').match(/T(\d{2}:\d{2})/);
  if (!endValue) return start;
  const end = new Date(`${entry.date}T${endValue[1]}:00`);
  return `${start} – ${new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit' }).format(end)}`;
}

export function FullMonthCalendar({
  year,
  month,
  entriesToShow = entries,
  onDayClick,
  onEntryEdit,
}: {
  year: number;
  month: number;
  entriesToShow?: typeof entries;
  onDayClick: () => void;
  onEntryEdit?: (entry: Entry) => void;
}) {
  const [selectedEntry, setSelectedEntry] = useState<Entry | null>(null);
  const monthStart = new Date(year, month, 1);
  const weekStart = getWeekStart() === 'monday' ? 1 : 0;
  const firstDayOffset = (monthStart.getDay() - weekStart + 7) % 7;
  const gridStart = new Date(year, month, 1 - firstDayOffset);
  const dates = Array.from({ length: 42 }, (_, index) => {
    const date = new Date(gridStart);
    date.setDate(gridStart.getDate() + index);
    return date;
  });
  const monthName = new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric' }).format(
    monthStart,
  );

  return (
    <div className="full-month-calendar">
      <div className="full-month-heading">
        <strong></strong>
        {['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
          .slice(weekStart)
          .concat(
            ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].slice(
              0,
              weekStart,
            ),
          )
          .map((day) => (
            <strong key={day}>{day}</strong>
          ))}
      </div>
      <div className="full-month-grid" aria-label={monthName}>
        {dates.map((date, index) => {
          const dateKey = formatDateKey(date);
          const dateEntries = entriesToShow.filter((entry) => entry.date === dateKey);
          const isCurrentMonth = date.getMonth() === month;
          const isToday = dateKey === formatDateKey(new Date());
          return (
            <div
              key={`${dateKey}-${index}`}
              className={`full-month-cell ${isCurrentMonth ? '' : 'is-outside'} ${isToday ? 'is-today' : ''} ${index % 7 >= 4 ? 'popover-left' : ''} ${index >= 21 ? 'popover-above' : ''}`}
              style={
                selectedEntry && dateEntries.some((entry) => entry.id === selectedEntry.id)
                  ? ({ '--entry-color': selectedEntry.color } as CSSProperties)
                  : undefined
              }
            >
              <button
                type="button"
                className="full-month-date"
                onClick={onDayClick}
                aria-label={`Create entry on ${date.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })}`}
              >
                {date.getDate()}
              </button>
              <div className="full-month-entries">
                {dateEntries.slice(0, 4).map((entry) => {
                  const allDay = isAllDayEvent(entry);
                  const time = formatEntryTime(entry);
                  return (
                    <button
                      type="button"
                      className={[
                        'full-month-entry',
                        allDay ? 'is-all-day-event' : `is-${entry.type}`,
                        entry.done ? 'is-done' : '',
                      ]
                        .filter(Boolean)
                        .join(' ')}
                      key={entry.id}
                      style={{ '--entry-color': entry.color } as CSSProperties}
                      title={entry.title}
                      onClick={() => setSelectedEntry(entry)}
                      aria-label={`${entry.title}${time ? `, ${time}` : ''}`}
                    >
                      {allDay ? (
                        <span className="full-month-entry-title">{entry.title}</span>
                      ) : entry.type === 'task' ? (
                        <>
                          <Square className="full-month-entry-icon" size={11} strokeWidth={2.5} aria-hidden="true" />
                          {time && <span className="full-month-entry-time">{time}</span>}
                          <span className="full-month-entry-title">{entry.title}</span>
                        </>
                      ) : (
                        <>
                          <span className="full-month-entry-dot" aria-hidden="true" />
                          {time && <span className="full-month-entry-time">{time}</span>}
                          <span className="full-month-entry-title">{entry.title}</span>
                        </>
                      )}
                    </button>
                  );
                })}
                {dateEntries.length > 4 && (
                  <span className="full-month-more">+{dateEntries.length - 4} more</span>
                )}
              </div>
              {selectedEntry && dateEntries.some((entry) => entry.id === selectedEntry.id) && (
                <section className="calendar-event-popover" aria-label="Entry details">
                  <button
                    type="button"
                    className="calendar-event-close"
                    onClick={() => setSelectedEntry(null)}
                    aria-label="Close entry details"
                  >
                    <X size={16} />
                  </button>
                  <div className="calendar-event-kind">
                    <span className="calendar-event-color" />
                    {selectedEntry.type === 'event' ? selectedEntry.list : 'Task'}
                    <button
                      type="button"
                      className="calendar-event-more"
                      onClick={() => onEntryEdit?.(selectedEntry)}
                      aria-label="Edit entry"
                    >
                      <MoreHorizontal size={18} />
                    </button>
                  </div>
                  <h3>{selectedEntry.title}</h3>
                  <p className="calendar-event-when">
                    {formatEntryRange(selectedEntry)} ·{' '}
                    {new Date(`${selectedEntry.date}T12:00:00`).toLocaleDateString('en-US', {
                      weekday: 'long',
                      month: 'long',
                      day: 'numeric',
                    })}
                  </p>
                  {selectedEntry.description && <p>{selectedEntry.description}</p>}
                  <button
                    type="button"
                    className="button button-primary calendar-event-edit"
                    onClick={() => onEntryEdit?.(selectedEntry)}
                  >
                    Open entry
                  </button>
                </section>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
