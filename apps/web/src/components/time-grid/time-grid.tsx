import type { CSSProperties, ReactNode } from 'react';

import { eachLocalDay, localDayKey } from './dates';
import { layoutOverlappingEvents } from './layout';
import { getCurrentTimePosition, positionInMinutes } from './position';
import { isAllDayOrMultiDay, segmentEventsByDay } from './segments';
import type { TimeGridEvent } from './types';

export interface TimeGridProps {
  rangeStart: Date;
  rangeEnd: Date;
  events: readonly TimeGridEvent[];
  now?: Date;
  startHour?: number;
  endHour?: number;
  pixelsPerMinute?: number;
  locale?: string;
  renderEvent?: (event: TimeGridEvent) => ReactNode;
  className?: string;
}

const rootStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'max-content minmax(0, 1fr)',
  overflow: 'auto',
};

const dayRowStyle: CSSProperties = {
  display: 'grid',
  gridAutoFlow: 'column',
  gridAutoColumns: 'minmax(0, 1fr)',
};

export function TimeGrid({
  rangeStart,
  rangeEnd,
  events,
  now,
  startHour = 0,
  endHour = 24,
  pixelsPerMinute = 1,
  locale,
  renderEvent = (event) => event.title,
  className,
}: TimeGridProps) {
  if (endHour <= startHour || startHour < 0 || endHour > 24) {
    throw new RangeError('Time grid hours must be within 0–24 and end after start.');
  }
  if (pixelsPerMinute <= 0) throw new RangeError('Pixels per minute must be positive.');

  const days = eachLocalDay(rangeStart, rangeEnd);
  if (days.length === 0) throw new RangeError('Date range must contain at least one day.');

  const range = { start: rangeStart, end: rangeEnd };
  const segments = segmentEventsByDay(events, range);
  const allDaySegments = segments.filter(({ event }) => isAllDayOrMultiDay(event));
  const timedSegments = segments.filter(({ event }) => !isAllDayOrMultiDay(event));
  const visibleStartMinute = startHour * 60;
  const visibleEndMinute = endHour * 60;
  const bodyHeight = (visibleEndMinute - visibleStartMinute) * pixelsPerMinute;
  const current = now
    ? getCurrentTimePosition(now, rangeStart, rangeEnd, visibleStartMinute, visibleEndMinute)
    : null;
  const dayFormatter = new Intl.DateTimeFormat(locale, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
  const timeFormatter = new Intl.DateTimeFormat(locale, {
    hour: 'numeric',
    minute: '2-digit',
  });
  const hours = Array.from({ length: endHour - startHour + 1 }, (_, index) => startHour + index);

  return (
    <section
      className={`time-grid-root ${className ?? ''}`}
      aria-label="Calendar time grid"
      style={rootStyle}
    >
      <span aria-hidden="true" />
      <div className="time-grid-days" role="row" style={dayRowStyle}>
        {days.map((day) => (
          <div id={`time-grid-${localDayKey(day)}`} role="columnheader" key={localDayKey(day)}>
            {dayFormatter.format(day)}
          </div>
        ))}
      </div>

      <div>All day</div>
      <div className="time-grid-days" aria-label="All-day events" role="row" style={dayRowStyle}>
        {days.map((day, dayIndex) => (
          <div
            aria-labelledby={`time-grid-${localDayKey(day)}`}
            role="gridcell"
            key={localDayKey(day)}
          >
            {allDaySegments
              .filter((segment) => segment.dayIndex === dayIndex)
              .map((segment) => (
                <article
                  aria-label={`${segment.event.title}, all day`}
                  key={`${segment.event.id}-${dayIndex}`}
                  style={{
                    borderInlineStart: `0.25rem solid ${segment.event.color ?? 'currentColor'}`,
                  }}
                >
                  {renderEvent(segment.event)}
                </article>
              ))}
          </div>
        ))}
      </div>

      <div aria-hidden="true" style={{ position: 'relative', height: bodyHeight }}>
        {hours.map((hour) => (
          <span
            key={hour}
            style={{
              position: 'absolute',
              top: (hour * 60 - visibleStartMinute) * pixelsPerMinute,
            }}
          >
            {timeFormatter.format(new Date(2000, 0, 1, hour))}
          </span>
        ))}
      </div>
      <div
        className="time-grid-days"
        aria-label="Timed events"
        role="grid"
        style={{ ...dayRowStyle, height: bodyHeight }}
      >
        {days.map((day, dayIndex) => {
          const daySegments = timedSegments.filter((segment) => segment.dayIndex === dayIndex);
          const columns = layoutOverlappingEvents(
            daySegments.map((segment) => ({
              ...segment.event,
              start: segment.start,
              end: segment.end,
            })),
          );

          return (
            <div
              aria-labelledby={`time-grid-${localDayKey(day)}`}
              role="gridcell"
              key={localDayKey(day)}
              style={{ position: 'relative', minWidth: 0 }}
            >
              {columns.map(({ event, column, columnCount }) => {
                const position = positionInMinutes(
                  event.start,
                  event.end,
                  day,
                  visibleStartMinute,
                  visibleEndMinute,
                );
                if (!position) return null;
                const width = 100 / columnCount;
                return (
                  <article
                    aria-label={`${event.title}, ${timeFormatter.format(event.start)} to ${timeFormatter.format(event.end)}`}
                    key={event.id}
                    style={{
                      position: 'absolute',
                      top: position.top * pixelsPerMinute,
                      height: position.height * pixelsPerMinute,
                      insetInlineStart: `${column * width}%`,
                      width: `${width}%`,
                      overflow: 'hidden',
                      borderInlineStart: `0.25rem solid ${event.color ?? 'currentColor'}`,
                    }}
                  >
                    {renderEvent(event)}
                  </article>
                );
              })}
              {current?.visible && current.dayIndex === dayIndex && (
                <div
                  aria-label={`Current time, ${timeFormatter.format(now)}`}
                  role="separator"
                  style={{
                    position: 'absolute',
                    top: current.top * pixelsPerMinute,
                    insetInline: 0,
                    borderBlockStart: '2px solid currentColor',
                    pointerEvents: 'none',
                  }}
                />
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
