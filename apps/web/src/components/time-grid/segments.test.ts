import { describe, expect, it } from 'vitest';

import { localDayKey } from './dates';
import { isAllDayOrMultiDay, segmentEventsByDay } from './segments';
import type { TimeGridEvent } from './types';

const event = (id: string, start: string, end: string, allDay = false): TimeGridEvent => ({
  id,
  title: id,
  start: new Date(start),
  end: new Date(end),
  allDay,
});

describe('segmentEventsByDay', () => {
  it('splits and clips multi-day events to the end-exclusive range', () => {
    const source = event('trip', '2026-09-13T18:00:00', '2026-09-16T10:00:00');
    const segments = segmentEventsByDay([source], {
      start: new Date('2026-09-14T00:00:00'),
      end: new Date('2026-09-16T00:00:00'),
    });

    expect(segments.map((segment) => localDayKey(segment.day))).toEqual([
      '2026-09-14',
      '2026-09-15',
    ]);
    expect(
      segments.map(({ startsBeforeDay, endsAfterDay }) => [startsBeforeDay, endsAfterDay]),
    ).toEqual([
      [true, true],
      [true, true],
    ]);
  });

  it('keeps explicit all-day events in the all-day class', () => {
    const source = event('holiday', '2026-09-14T00:00:00', '2026-09-15T00:00:00', true);
    expect(isAllDayOrMultiDay(source)).toBe(true);
  });
});
