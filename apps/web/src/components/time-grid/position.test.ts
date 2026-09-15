import { describe, expect, it } from 'vitest';

import { getCurrentTimePosition, positionInMinutes } from './position';

describe('minute positioning', () => {
  it('clips an event to the visible minute window', () => {
    expect(
      positionInMinutes(
        new Date('2026-09-14T07:30:00'),
        new Date('2026-09-14T09:15:00'),
        new Date('2026-09-14T00:00:00'),
        8 * 60,
        18 * 60,
      ),
    ).toEqual({ top: 0, height: 75 });
  });

  it('returns null for an event outside the day', () => {
    expect(
      positionInMinutes(
        new Date('2026-09-13T09:00:00'),
        new Date('2026-09-13T10:00:00'),
        new Date('2026-09-14T00:00:00'),
      ),
    ).toBeNull();
  });
});

describe('getCurrentTimePosition', () => {
  it('returns the day, minute offset, and visibility', () => {
    expect(
      getCurrentTimePosition(
        new Date('2026-09-15T09:30:00'),
        new Date('2026-09-14T00:00:00'),
        new Date('2026-09-21T00:00:00'),
        8 * 60,
        18 * 60,
      ),
    ).toEqual({ dayIndex: 1, minute: 570, top: 90, visible: true });
  });

  it('returns null outside the supplied date range', () => {
    expect(
      getCurrentTimePosition(
        new Date('2026-09-21T09:30:00'),
        new Date('2026-09-14T00:00:00'),
        new Date('2026-09-21T00:00:00'),
      ),
    ).toBeNull();
  });
});
