import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { Entry } from '../data';
import { FullMonthCalendar } from './full-month-calendar';

const entry = (id: string, date: string, title: string, overrides: Partial<Entry> = {}): Entry => ({
  id,
  date,
  title,
  description: '',
  containerId: 'calendar-1',
  list: 'Calendar',
  color: '#123456',
  type: 'event',
  ...overrides,
});

describe('FullMonthCalendar', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows entries only in the rendered six-week date range', () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn(() => 'sunday') });
    const html = renderToStaticMarkup(
      <FullMonthCalendar
        year={2025}
        month={0}
        entriesToShow={[
          entry('before', '2024-12-28', 'Before grid'),
          entry('first', '2024-12-29', 'First grid day'),
          entry('middle', '2025-01-15', 'Middle of month'),
          entry('last', '2025-02-08', 'Last grid day'),
          entry('after', '2025-02-09', 'After grid'),
        ]}
        onDayClick={() => undefined}
      />,
    );

    expect(html).toContain('First grid day');
    expect(html).toContain('Middle of month');
    expect(html).toContain('Last grid day');
    expect(html).not.toContain('Before grid');
    expect(html).not.toContain('After grid');
  });

  it('renders all-day events as filled bars and timed entries with type-specific markers', () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn(() => 'sunday') });
    const html = renderToStaticMarkup(
      <FullMonthCalendar
        year={2025}
        month={0}
        entriesToShow={[
          entry('all-day', '2025-01-15', 'All-day event', {
            fields: { all_day: true, start_date: '2025-01-15T00:00:00.000Z' },
          }),
          entry('timed', '2025-01-15', 'Timed event', {
            fields: { all_day: false, start_date: '2025-01-15T09:00:00.000Z' },
          }),
          entry('task', '2025-01-15', 'Task', {
            type: 'task',
            fields: { due_date: '2025-01-15T13:30:00.000Z' },
          }),
        ]}
        onDayClick={() => undefined}
      />,
    );

    expect(html).toContain('class="full-month-entry is-all-day-event"');
    expect(html).toContain('style="background-color:#123456"');
    expect(html).toContain('class="full-month-entry is-event"');
    expect(html).toContain('class="full-month-entry-dot" style="background-color:#123456"');
    expect(html).toContain('class="full-month-entry is-task"');
    expect(html).toContain('full-month-entry-time">1:30 PM</span>');
  });

  it('uses the saved week start for headings and grid boundaries', () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn(() => 'monday') });
    const html = renderToStaticMarkup(
      <FullMonthCalendar
        year={2025}
        month={0}
        entriesToShow={[
          entry('sunday', '2024-12-29', 'Prior Sunday'),
          entry('monday', '2024-12-30', 'Prior Monday'),
        ]}
        onDayClick={() => undefined}
      />,
    );

    expect(html.indexOf('Mon')).toBeLessThan(html.indexOf('Sun'));
    expect(html).toContain('Prior Monday');
    expect(html).not.toContain('Prior Sunday');
  });
});
