import { useEffect, useState } from 'react';
import { MoreHorizontal, Plus } from 'lucide-react';
import { entries, lists, type Entry } from '../../data';
import { EntryRow } from '../entry-row';

export function ListsTab({
  onAdd,
  onEdit,
  onChanged,
}: {
  onAdd: () => void;
  onEdit: (entry: Entry) => void;
  onChanged: () => void;
}) {
  const taskLists = lists.filter((list) => list.kind === 'list');
  const [selected, setSelected] = useState('');
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
  return (
    <>
      <header className="page-header">
        <h1>Lists</h1>
        <button className="button button-primary" onClick={onAdd}>
          <Plus size={17} /> Add list
        </button>
      </header>
      <div className="lists-layout">
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
        <section className="selected-list">
          <div className="selected-list-header">
            <div>
              <span className="list-heading">
                <i style={{ background: current.color }} />
                {current.name}
              </span>
              <p>{current.description}</p>
            </div>
            <button className="icon-button">
              <MoreHorizontal size={19} />
            </button>
          </div>
          {listEntries.map((entry) => (
            <EntryRow entry={entry} key={entry.id} onEdit={onEdit} onChanged={onChanged} />
          ))}
        </section>
      </div>
    </>
  );
}
