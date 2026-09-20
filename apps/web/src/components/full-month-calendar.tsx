import { Fragment } from 'react';
import { Square } from 'lucide-react';
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

function getISOWeekNumber(date: Date) {
  const thursday = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()));
  thursday.setUTCDate(thursday.getUTCDate() + 3 - ((thursday.getUTCDay() + 6) % 7));
  const firstThursday = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 4));
  return 1 + Math.round((thursday.getTime() - firstThursday.getTime()) / 604800000);
}

export function FullMonthCalendar({
  year,
  month,
  entriesToShow = entries,
  onDayClick,
}: {
  year: number;
  month: number;
  entriesToShow?: typeof entries;
  onDayClick: () => void;
}) {
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
        {['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
          .slice(weekStart)
          .concat(['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].slice(0, weekStart))
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
          const isWeekStart = (date.getDay() - weekStart + 7) % 7 === 0;
          return (
            <Fragment key={`${dateKey}-${index}`}>
              {isWeekStart && (
                <span className="calendar-week-gutter" aria-hidden="true">
                  {getISOWeekNumber(date)}
                </span>
              )}
              <div
                className={`full-month-cell ${isCurrentMonth ? '' : 'is-outside'} ${isToday ? 'is-today' : ''}`}
                role="button"
                tabIndex={0}
                onClick={onDayClick}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    onDayClick();
                  }
                }}
              >
                <span className="full-month-date">{date.getDate()}</span>
                <div className="full-month-entries">
                  {dateEntries.map((entry) => {
                    const allDay = isAllDayEvent(entry);
                    const time = formatEntryTime(entry);
                    return (
                      <div
                        className={`full-month-entry ${allDay ? 'is-all-day-event' : `is-${entry.type}`}`}
                        key={entry.id}
                        style={allDay ? { backgroundColor: entry.color } : undefined}
                        title={entry.title}
                      >
                        {allDay ? (
                          <span className="full-month-entry-title">{entry.title}</span>
                        ) : entry.type === 'task' ? (
                          <>
                            <Square
                              className="full-month-entry-icon"
                              style={{ color: entry.color }}
                              size={11}
                              strokeWidth={2.5}
                              aria-hidden="true"
                            />
                            {time && <span className="full-month-entry-time">{time}</span>}
                            <span className="full-month-entry-title">{entry.title}</span>
                          </>
                        ) : (
                          <>
                            <span
                              className="full-month-entry-dot"
                              style={{ backgroundColor: entry.color }}
                              aria-hidden="true"
                            />
                            {time && <span className="full-month-entry-time">{time}</span>}
                            <span className="full-month-entry-title">{entry.title}</span>
                          </>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            </Fragment>
          );
        })}
      </div>
    </div>
  );
}
