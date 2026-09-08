import { useState } from 'react';
import { MoreHorizontal, Plus } from 'lucide-react';
import { entries, lists } from '../../data';
import { EntryRow } from '../entry-row';

export function ListsTab({ onAdd }: { onAdd: () => void }) {
  const [selected, setSelected] = useState('Work');
  const current = lists.find((list) => list.name === selected) ?? {
    name: 'No lists yet',
    color: 'var(--muted)',
    count: 0,
    description: 'Create a list to organize your entries',
  };
  const listEntries = entries.filter((entry) => entry.list === selected);
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
          {lists.map((list) => (
            <button
              className={`list-picker-item ${selected === list.name ? 'active' : ''}`}
              onClick={() => setSelected(list.name)}
              key={list.name}
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
            <EntryRow entry={entry} key={entry.id} />
          ))}
        </section>
      </div>
    </>
  );
}
