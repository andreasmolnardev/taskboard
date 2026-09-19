import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { entries, type Entry } from '../data';
import { CalendarMonth } from './calendar-month';

const originalEntries = [...entries];

const entry = (id: string, date: string): Entry => ({
  id,
  date,
  title: id,
  description: '',
  containerId: 'list-1',
  list: 'List',
  color: '#123456',
  type: 'task',
});

describe('CalendarMonth', () => {
  afterEach(() => {
    entries.splice(0, entries.length, ...originalEntries);
    vi.unstubAllGlobals();
  });

  it('marks distinct days in the requested month only', () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn(() => 'sunday') });
    entries.splice(
      0,
      entries.length,
      entry('first', '2025-03-07'),
      entry('same-day', '2025-03-07'),
      entry('second', '2025-03-21'),
      entry('previous-month', '2025-02-28'),
      entry('next-month', '2025-04-01'),
    );

    const html = renderToStaticMarkup(<CalendarMonth year={2025} month={2} />);

    expect(html).toContain('March 2025');
    expect(html.match(/<i><\/i>/g)).toHaveLength(2);
  });

  it('shows an accessible expand control when supplied', () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn(() => 'sunday') });

    const html = renderToStaticMarkup(
      <CalendarMonth year={2025} month={2} onExpand={() => undefined} />,
    );

    expect(html).toContain('aria-label="Open March 2025 in calendar"');
  });
});
