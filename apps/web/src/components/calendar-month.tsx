import { Maximize2 } from 'lucide-react';
import { entries, getWeekStart } from '../data';

export function CalendarMonth({
  year,
  month,
  onExpand,
}: {
  year: number;
  month: number;
  onExpand?: () => void;
}) {
  const weekStart = getWeekStart() === 'monday' ? 1 : 0;
  const firstDay = (new Date(year, month, 1).getDay() - weekStart + 7) % 7;
  const days = new Date(year, month + 1, 0).getDate();
  const monthEntries = entries.filter((entry) => {
    const date = new Date(`${entry.date}T12:00:00`);
    return date.getFullYear() === year && date.getMonth() === month;
  });
  const markedDays = new Set(
    monthEntries.map((entry) => new Date(`${entry.date}T12:00:00`).getDate()),
  );
  const today = new Date();
  const isCurrentMonth = today.getFullYear() === year && today.getMonth() === month;
  const monthName = new Intl.DateTimeFormat('en-US', {
    month: 'long',
    year: 'numeric',
  }).format(new Date(year, month, 1));

  return (
    <div className="month-card">
      <div className="month-card-heading">
        <h3>{monthName}</h3>
        {onExpand && (
          <button
            type="button"
            className="month-expand-button"
            onClick={onExpand}
            aria-label={`Open ${monthName} in calendar`}
            title={`Open ${monthName} in calendar`}
          >
            <Maximize2 size={16} />
          </button>
        )}
      </div>
      <div className="weekdays">
        {['S', 'M', 'T', 'W', 'T', 'F', 'S']
          .slice(weekStart)
          .concat(['S', 'M', 'T', 'W', 'T', 'F', 'S'].slice(0, weekStart))
          .map((day, index) => (
            <span key={`${day}-${index}`}>{day}</span>
          ))}
      </div>
      <div className="calendar-grid">
        {Array.from({ length: firstDay }, (_, index) => (
          <span className="calendar-empty" key={`empty-${index}`} />
        ))}
        {Array.from({ length: days }, (_, index) => {
          const day = index + 1;
          const isToday = isCurrentMonth && day === today.getDate();
          return (
            <span className={`calendar-day ${isToday ? 'today' : ''}`} key={day}>
              <span>{day}</span>
              {markedDays.has(day) && <i />}
            </span>
          );
        })}
      </div>
    </div>
  );
}
