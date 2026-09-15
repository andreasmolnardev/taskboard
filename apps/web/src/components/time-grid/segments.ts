import { addLocalDays, localDayIndex, startOfLocalDay } from './dates';
import type { DateRange, EventSegment, TimeGridEvent } from './types';

/** Splits events at local midnight and clips them to an end-exclusive date range. */
export function segmentEventsByDay(
  events: readonly TimeGridEvent[],
  range: DateRange,
): EventSegment[] {
  const rangeStart = startOfLocalDay(range.start);
  const rangeEnd = startOfLocalDay(range.end);
  if (rangeEnd <= rangeStart) throw new RangeError('Date range end must be after its start.');

  const segments: EventSegment[] = [];
  for (const event of events) {
    if (event.end <= event.start) {
      throw new RangeError(`Event "${event.id}" must end after it starts.`);
    }

    const clippedStart = event.start > rangeStart ? event.start : rangeStart;
    const clippedEnd = event.end < rangeEnd ? event.end : rangeEnd;
    if (clippedEnd <= clippedStart) continue;

    for (let day = startOfLocalDay(clippedStart); day < clippedEnd; day = addLocalDays(day, 1)) {
      const nextDay = addLocalDays(day, 1);
      const start = clippedStart > day ? clippedStart : day;
      const end = clippedEnd < nextDay ? clippedEnd : nextDay;
      if (end <= start) continue;

      segments.push({
        event,
        day,
        start,
        end,
        dayIndex: localDayIndex(day, rangeStart),
        startsBeforeDay: event.start < day,
        endsAfterDay: event.end > nextDay,
      });
    }
  }

  return segments.sort(
    (left, right) =>
      left.dayIndex - right.dayIndex ||
      left.start.getTime() - right.start.getTime() ||
      left.event.id.localeCompare(right.event.id),
  );
}

export function isAllDayOrMultiDay(event: TimeGridEvent): boolean {
  return event.allDay === true || startOfLocalDay(event.start) < startOfLocalDay(event.end);
}
