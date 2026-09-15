import { describe, expect, it } from 'vitest';

import { localToRecord, recordToLocal } from './create-composer.helpers';

describe('create composer date helpers', () => {
  it('converts compact iCalendar dates to valid date input values', () => {
    expect(recordToLocal('20260914', true)).toBe('2026-09-14');
    expect(recordToLocal('20260914', false)).toBe('2026-09-14T00:00');
    expect(recordToLocal('20260914T093000', false)).toBe('2026-09-14T09:30');
    expect(recordToLocal('20260914T093000', true)).toBe('2026-09-14');
    expect(recordToLocal('2026-09-14 09:30:00.000Z', false)).toBe('2026-09-14T09:30');
  });

  it('creates a record value from a datetime-local field', () => {
    expect(localToRecord('2026-09-14T09:30', false)).toMatchObject({
      local: '20260914T093000',
      mode: 'zoned',
    });
  });

  it('normalizes a date when switching between all-day and timed values', () => {
    expect(localToRecord('2026-09-14T09:30', true)).toMatchObject({
      local: '20260914',
      mode: 'date',
    });
    expect(localToRecord('2026-09-14', false)).toMatchObject({
      local: '20260914T000000',
      mode: 'zoned',
    });
    expect(localToRecord('20260914', false)).toMatchObject({
      local: '20260914T000000',
      mode: 'zoned',
    });
  });
});
