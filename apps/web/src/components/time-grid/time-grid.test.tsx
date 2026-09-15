import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';

import { TimeGrid } from './time-grid';
import type { TimeGridEvent } from './types';

const events: TimeGridEvent[] = [
  {
    id: 'standup',
    title: 'Team standup',
    start: new Date('2026-09-14T09:00:00'),
    end: new Date('2026-09-14T09:30:00'),
    color: 'blue',
  },
  {
    id: 'conference',
    title: 'Conference',
    start: new Date('2026-09-14T00:00:00'),
    end: new Date('2026-09-16T00:00:00'),
    allDay: true,
  },
];

describe('TimeGrid', () => {
  it('renders accessible headers, events, and current time for a supplied range', () => {
    const html = renderToStaticMarkup(
      <TimeGrid
        rangeStart={new Date('2026-09-14T00:00:00')}
        rangeEnd={new Date('2026-09-16T00:00:00')}
        events={events}
        now={new Date('2026-09-14T10:00:00')}
        startHour={8}
        endHour={18}
        locale="en-US"
      />,
    );

    expect(html).toContain('aria-label="Calendar time grid"');
    expect(html).toContain('Mon, Sep 14');
    expect(html).toContain('Tue, Sep 15');
    expect(html).toContain('aria-label="Team standup, 9:00 AM to 9:30 AM"');
    expect(html.match(/aria-label="Conference, all day"/g)).toHaveLength(2);
    expect(html).toContain('aria-label="Current time, 10:00 AM"');
  });

  it('supports custom event content', () => {
    const html = renderToStaticMarkup(
      <TimeGrid
        rangeStart={new Date('2026-09-14T00:00:00')}
        rangeEnd={new Date('2026-09-15T00:00:00')}
        events={events}
        renderEvent={(event) => <strong>{event.title}</strong>}
      />,
    );

    expect(html).toContain('<strong>Team standup</strong>');
  });
});
