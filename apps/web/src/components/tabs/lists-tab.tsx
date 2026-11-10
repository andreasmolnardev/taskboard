import { useEffect, useState } from 'react';
import { MoreHorizontal, Plus } from 'lucide-react';
import { entries, getActiveContainers, lists, type Entry } from '../../data';
import { EntryRow } from '../entry-row';
import { ResourceDataView } from '../patterns/resource-data-view';
import { ResourceQueryControls } from '../patterns/resource-query-controls';
import { renderLayout } from '../../lib/slopstack';
import type { ResourceQuery } from '../../lib/slopstack/resource-client';
import { taskResource } from '../../resources/taskboard';
import { taskboardLayout } from '../../layouts/taskboard-layout';

export function ListsTab({
  onAdd,
  onEdit,
  onChanged,
}: {
  onAdd: () => void;
  onEdit: (entry: Entry) => void;
  onChanged: () => void;
}) {
  const taskLists = getActiveContainers(lists).filter((list) => list.kind === 'list');
  const [selected, setSelected] = useState('');
  const [query, setQuery] = useState<ResourceQuery>({ sort: taskResource.query?.defaultSort });
  useEffect(() => {
    if (!taskLists.some((list) => list.id === selected)) setSelected(taskLists[0]?.id ?? '');
  }, [selected, taskLists]);
  const current = taskLists.find((list) => list.id === selected) ?? {
    name: 'No lists yet',
    color: 'var(--muted)',
    count: 0,
    description: 'Create a list to organize your entries',
  };
  const listEntries = entries.filter((entry) => entry.containerId === selected);
  const filteredEntries = listEntries
    .filter(
      (entry) =>
        !query.search ||
        `${entry.title} ${entry.description}`
          .toLocaleLowerCase()
          .includes(query.search.toLocaleLowerCase()),
    )
    .filter((entry) => {
      const status = query.filters?.status;
      return !status || (status === 'COMPLETED' ? entry.done : entry.fields?.status === status);
    })
    .sort((a, b) => {
      if (query.sort === 'title') return a.title.localeCompare(b.title);
      if (query.sort === '-updated')
        return String(b.fields?.updated ?? '').localeCompare(String(a.fields?.updated ?? ''));
      return a.date.localeCompare(b.date) || (a.time ?? '').localeCompare(b.time ?? '');
    });
  const master = (
    <div className="list-picker">
      {taskLists.map((list) => (
        <button
          className={`list-picker-item ${selected === list.id ? 'active' : ''}`}
          onClick={() => setSelected(list.id)}
          key={list.id}
        >
          <i style={{ background: list.color }} />
          <span>
            <strong>{list.name}</strong>
            <small>{list.description}</small>
          </span>
          <em>{list.count}</em>
        </button>
      ))}
    </div>
  );
  const detail = (
    <section className="selected-list">
      <div className="selected-list-header">
        <div>
          <span className="list-heading">
            <i style={{ background: current.color }} />
            {current.name}
          </span>
          <p>{current.description}</p>
        </div>
        <button className="icon-button" aria-label="List options">
          <MoreHorizontal size={19} />
        </button>
      </div>
      <ResourceQueryControls
        resource={taskResource}
        value={query}
        onChange={setQuery}
        total={filteredEntries.length}
      />
      <ResourceDataView
        rows={filteredEntries}
        columns={[{ id: 'title', header: 'Task', cell: (entry) => entry.title }]}
        getRowKey={(entry) => entry.id}
        mode="list"
        emptyMessage="No tasks match this view."
        renderItem={(entry) => <EntryRow entry={entry} onEdit={onEdit} onChanged={onChanged} />}
      />
    </section>
  );
  return (
    <>
      <header className="page-header">
        <h1>Lists</h1>
        <button className="button button-primary" onClick={onAdd}>
          <Plus size={17} /> Add list
        </button>
      </header>
      {renderLayout(taskboardLayout, {
        master: { kind: 'component', content: master },
        detail: { kind: 'component', content: detail },
      })}
    </>
  );
}
