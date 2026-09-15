import { describe, expect, it } from 'vitest';

import { layoutOverlappingEvents } from './layout';
import type { TimeGridEvent } from './types';

const event = (id: string, start: string, end: string): TimeGridEvent => ({
  id,
  title: id,
  start: new Date(start),
  end: new Date(end),
});

describe('layoutOverlappingEvents', () => {
  it('assigns stable columns and the peak count to a connected overlap group', () => {
    const result = layoutOverlappingEvents([
      event('long', '2026-09-14T09:00:00', '2026-09-14T12:00:00'),
      event('early', '2026-09-14T09:30:00', '2026-09-14T10:00:00'),
      event('late', '2026-09-14T10:00:00', '2026-09-14T11:00:00'),
    ]);

    expect(result.map(({ event, column, columnCount }) => [event.id, column, columnCount])).toEqual(
      [
        ['long', 0, 2],
        ['early', 1, 2],
        ['late', 1, 2],
      ],
    );
  });

  it('starts a new one-column group when event edges only touch', () => {
    const result = layoutOverlappingEvents([
      event('first', '2026-09-14T09:00:00', '2026-09-14T10:00:00'),
      event('second', '2026-09-14T10:00:00', '2026-09-14T11:00:00'),
    ]);

    expect(result.map(({ column, columnCount }) => [column, columnCount])).toEqual([
      [0, 1],
      [0, 1],
    ]);
  });

  it('rejects zero-length events', () => {
    expect(() =>
      layoutOverlappingEvents([event('bad', '2026-09-14T09:00:00', '2026-09-14T09:00:00')]),
    ).toThrow(RangeError);
  });
});
