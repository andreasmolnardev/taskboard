import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';

import {
  ContainerManager,
  getSafeRemovalAction,
  resolveDefaultSelection,
  type ManagedContainer,
} from './container-manager';

const containers: ManagedContainer[] = [
  {
    id: 'personal',
    name: 'Personal',
    color: '#2f6f63',
    description: 'Home tasks',
    kind: 'list',
    count: 0,
    visible: true,
    archived: false,
  },
  {
    id: 'work',
    name: 'Work',
    color: '#3b6ea8',
    description: 'Team events',
    kind: 'calendar',
    count: 4,
    visible: false,
    archived: false,
  },
  {
    id: 'old',
    name: 'Old calendar',
    color: '#7657a8',
    description: 'Archived events',
    kind: 'calendar',
    count: 2,
    visible: false,
    archived: true,
  },
];

describe('ContainerManager', () => {
  it('renders lists and calendars with accessible controls', () => {
    const html = renderToStaticMarkup(
      <ContainerManager
        containers={containers}
        defaultSelectedId="work"
        onUpdate={vi.fn()}
        onVisibilityChange={vi.fn()}
        onArchiveChange={vi.fn()}
        onRemove={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="Lists and calendars"');
    expect(html).toContain('aria-label="Select list Personal"');
    expect(html).toContain('aria-label="Select calendar Work"');
    expect(html).toContain('aria-label="Edit Personal"');
    expect(html).toContain('aria-label="Remove Work"');
    expect(html).toContain('aria-label="Unarchive Old calendar"');
    expect(html).toContain('Archived');
    expect(html).toContain('aria-label="Show Personal"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain('Team events');
  });

  it('falls back to the first container for default selection', () => {
    expect(resolveDefaultSelection(containers, 'missing')).toBe('personal');
    expect(resolveDefaultSelection(containers, 'work')).toBe('work');
    expect(resolveDefaultSelection([], 'work')).toBeUndefined();
  });

  it('archives non-empty containers and deletes empty containers', () => {
    expect(getSafeRemovalAction(containers[0])).toBe('delete');
    expect(getSafeRemovalAction(containers[1])).toBe('archive');
  });

  it('renders a clear empty state', () => {
    const html = renderToStaticMarkup(
      <ContainerManager
        containers={[]}
        onUpdate={vi.fn()}
        onVisibilityChange={vi.fn()}
        onArchiveChange={vi.fn()}
        onRemove={vi.fn()}
      />,
    );

    expect(html).toContain('No lists or calendars yet.');
  });
});
