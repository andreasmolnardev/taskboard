import { afterEach, describe, expect, it, vi } from 'vitest';

import { formatDate, formatDateKey, getWeekStart, weekStartStorageKey } from './data';

describe('date helpers', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('formats a local date key with zero-padded month and day', () => {
    expect(formatDateKey(new Date(2025, 0, 9, 23, 45))).toBe('2025-01-09');
  });

  it('keeps the local calendar day near a UTC boundary', () => {
    const date = new Date(2025, 5, 1, 0, 5);

    expect(formatDateKey(date)).toBe(
      `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`,
    );
  });

  it('formats stored date-only values without shifting the day', () => {
    expect(formatDate('2024-02-29')).toBe('Thursday, February 29');
  });
});

describe('getWeekStart', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('uses Monday only for the saved Monday preference', () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn((key: string) => (key === weekStartStorageKey ? 'monday' : null)),
    });

    expect(getWeekStart()).toBe('monday');
  });

  it.each([null, '', 'saturday'])('falls back to Sunday for %s', (savedValue) => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => savedValue),
    });

    expect(getWeekStart()).toBe('sunday');
  });
});
