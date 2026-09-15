import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../entry-row', () => ({ EntryRow: () => null }));

import { lists } from '../../data';
import { ListsTab } from './lists-tab';

const originalLists = [...lists];

describe('ListsTab', () => {
  afterEach(() => {
    lists.splice(0, lists.length, ...originalLists);
  });

  it('offers task lists but excludes calendars from the list picker', () => {
    lists.splice(
      0,
      lists.length,
      {
        id: 'tasks',
        name: 'Work tasks',
        color: '#123456',
        count: 2,
        description: 'Things to do',
        kind: 'list',
        archived: false,
      },
      {
        id: 'events',
        name: 'Work calendar',
        color: '#654321',
        count: 3,
        description: 'Places to be',
        kind: 'calendar',
        archived: false,
      },
      {
        id: 'old-tasks',
        name: 'Archived tasks',
        color: '#7657a8',
        count: 1,
        description: 'Old tasks',
        kind: 'list',
        archived: true,
      },
    );

    const html = renderToStaticMarkup(
      <ListsTab onAdd={() => undefined} onEdit={() => undefined} onChanged={() => undefined} />,
    );

    expect(html).toContain('Work tasks');
    expect(html).toContain('Things to do');
    expect(html).not.toContain('Work calendar');
    expect(html).not.toContain('Places to be');
    expect(html).not.toContain('Archived tasks');
  });
});
