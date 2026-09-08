import { Fragment } from 'react';
import { entries, formatDateKey, getWeekStart } from '../data';

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
        <strong>Week</strong>
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
              {isWeekStart && <span className="calendar-week-gutter" aria-hidden="true">{getISOWeekNumber(date)}</span>}
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
                  {dateEntries.map((entry) => (
                    <div
                      className="full-month-entry"
                      key={entry.id}
                      style={{ backgroundColor: entry.color }}
                      title={entry.title}
                    >
                      {entry.title}
                    </div>
                  ))}
                </div>
              </div>
            </Fragment>
          );
        })}
      </div>
    </div>
  );
}
