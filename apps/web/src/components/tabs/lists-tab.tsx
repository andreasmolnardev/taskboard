import { useEffect, useState } from 'react';
import { Kanban, List, MoreHorizontal, Plus } from 'lucide-react';
import { entries, getActiveContainers, lists, type Entry, type EntryContainer } from '../../data';
import { EntryRow } from '../entry-row';
import { ResourceDataView } from '../patterns/resource-data-view';
import { ResourceQueryControls } from '../patterns/resource-query-controls';
import { renderLayout } from '../../lib/slopstack';
import type { ResourceQuery } from '../../lib/slopstack/resource-client';
import { taskResource } from '../../resources/taskboard';
import { taskboardLayout } from '../../layouts/taskboard-layout';
import { offlineCollection } from '../../offline';

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
  const [projectView, setProjectView] = useState<'list' | 'board'>('list');
  const [query, setQuery] = useState<ResourceQuery>({ sort: taskResource.query?.defaultSort });
  useEffect(() => {
    if (!taskLists.some((list) => list.id === selected)) setSelected(taskLists[0]?.id ?? '');
  }, [selected, taskLists]);
  useEffect(() => {
    try {
      setProjectView(localStorage.getItem(`taskboard-project-view:${selected}`) === 'board' ? 'board' : 'list');
    } catch {
      setProjectView('list');
    }
  }, [selected]);
  const current: EntryContainer = taskLists.find((list) => list.id === selected) ?? {
    id: '',
    name: 'No lists yet',
    color: 'var(--muted)',
    count: 0,
    description: 'Create a list to organize your entries',
    kind: 'list',
    archived: false,
  };
  const listEntries = entries.filter((entry) => entry.containerId === selected);
  const projectTasks = listEntries.filter((entry) => entry.type === 'task');
  const completedTasks = projectTasks.filter((entry) => entry.done).length;
  const projectProgress = projectTasks.length
    ? Math.round((completedTasks / projectTasks.length) * 100)
    : 0;
  const today = new Date().toISOString().slice(0, 10);
  const upcomingDeadlines = projectTasks.filter((entry) => entry.date >= today && !entry.done).length;
  const overdueTasks = projectTasks.filter((entry) => entry.date && entry.date < today && !entry.done).length;
  const updateProject = async (patch: Record<string, unknown>) => {
    if (!selected) return;
    await offlineCollection('lists').update(selected, patch);
    onChanged();
  };
  const setTaskStatus = async (entry: Entry, status: 'NEEDS-ACTION' | 'IN-PROCESS' | 'COMPLETED') => {
    await offlineCollection('todos').update(entry.id, {
      status,
      completed: status === 'COMPLETED',
      percent_complete: status === 'COMPLETED' ? '100' : '0',
    });
    onChanged();
  };
  const taskStatus = (entry: Entry) =>
    entry.done || entry.fields?.status === 'COMPLETED'
      ? 'COMPLETED'
      : entry.fields?.status === 'IN-PROCESS'
        ? 'IN-PROCESS'
        : 'NEEDS-ACTION';
  const orderedTasks = (status: 'NEEDS-ACTION' | 'IN-PROCESS' | 'COMPLETED') =>
    boardTasks
      .filter((entry) => taskStatus(entry) === status)
      .sort((a, b) => Number(a.fields?.sort_order ?? 0) - Number(b.fields?.sort_order ?? 0));
  const persistOrder = async (items: Entry[]) => {
    await Promise.all(items.map((entry, index) =>
      offlineCollection('todos').update(entry.id, { sort_order: index + 1 }),
    ));
    onChanged();
  };
  const reorderTask = async (entry: Entry, direction: -1 | 1) => {
    const group = orderedTasks(taskStatus(entry));
    const index = group.findIndex((item) => item.id === entry.id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= group.length) return;
    [group[index], group[target]] = [group[target], group[index]];
    await persistOrder(group);
  };
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
  const boardTasks = filteredEntries.filter((entry) => entry.type === 'task');
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
        {!current.isProject && selected && (
          <button type="button" onClick={() => void updateProject({ is_project: true })}>
            Make project
          </button>
        )}
      </div>
      {current.isProject && (
        <section className="project-overview" aria-label="Project overview">
          <div className="project-overview-header">
            <strong>Project</strong>
            <button type="button" onClick={() => void updateProject({ is_project: false })}>
              Turn off project mode
            </button>
            <label>
              Status
              <select
                aria-label="Project status"
                value={current.projectStatus ?? 'active'}
                onChange={(event) => void updateProject({ project_status: event.target.value })}
              >
                <option value="active">Active</option>
                <option value="paused">Paused</option>
                <option value="completed">Completed</option>
              </select>
            </label>
          </div>
          <label>
            Project home
            <input
              aria-label="Project home URL"
              type="url"
              defaultValue={current.homeUrl ?? ''}
              placeholder="https://"
              onBlur={(event) => {
                if (event.target.value !== (current.homeUrl ?? ''))
                  void updateProject({ home_url: event.target.value });
              }}
            />
          </label>
          {current.homeUrl && (
            <a href={current.homeUrl} target="_blank" rel="noreferrer">
              Open project home
            </a>
          )}
          <label>
            Target date
            <input
              aria-label="Project target date"
              type="date"
              value={current.targetDate ?? ''}
              onChange={(event) => void updateProject({ target_date: event.target.value })}
            />
          </label>
          <div>
            <span>{completedTasks} of {projectTasks.length} tasks done · {projectProgress}%</span>
            <progress aria-label="Project completion" max={100} value={projectProgress} />
          </div>
          <p>{upcomingDeadlines} upcoming · {overdueTasks} overdue</p>
        </section>
      )}
      <ResourceQueryControls
        resource={taskResource}
        value={query}
        onChange={setQuery}
        total={filteredEntries.length}
      />
      {current.isProject && (
        <div className="project-view-toggle" role="group" aria-label="Project task view">
          <button
            type="button"
            aria-pressed={projectView === 'list'}
            onClick={() => {
              setProjectView('list');
              localStorage.setItem(`taskboard-project-view:${selected}`, 'list');
            }}
          ><List size={16} /> List</button>
          <button
            type="button"
            aria-pressed={projectView === 'board'}
            onClick={() => {
              setProjectView('board');
              localStorage.setItem(`taskboard-project-view:${selected}`, 'board');
            }}
          ><Kanban size={16} /> Board</button>
        </div>
      )}
      {current.isProject && projectView === 'board' ? (
        <div className="project-board" aria-label="Tasks by status">
          {([
            ['NEEDS-ACTION', 'Todo'],
            ['IN-PROCESS', 'In progress'],
            ['COMPLETED', 'Done'],
          ] as const).map(([status, label]) => (
            <section
              key={status}
              aria-label={label}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault();
                const moved = boardTasks.find((entry) => entry.id === event.dataTransfer.getData('text/taskboard-task'));
                if (moved && taskStatus(moved) !== status) {
                  const targetGroup = orderedTasks(status);
                  void Promise.all([
                    setTaskStatus(moved, status),
                    offlineCollection('todos').update(moved.id, { sort_order: targetGroup.length + 1 }),
                  ]);
                }
              }}
            >
              <h3>{label} ({boardTasks.filter((entry) => taskStatus(entry) === status).length})</h3>
              {orderedTasks(status).map((entry, index, items) => (
                <article
                  className="project-board-task"
                  key={entry.id}
                  draggable
                  onDragStart={(event) => event.dataTransfer.setData('text/taskboard-task', entry.id)}
                >
                  <button type="button" onClick={() => onEdit(entry)}>{entry.title}</button>
                  {entry.date && <small>{entry.date}</small>}
                  <select
                    aria-label={`Status for ${entry.title}`}
                    value={status}
                    onChange={(event) => void setTaskStatus(entry, event.target.value as typeof status)}
                  >
                    <option value="NEEDS-ACTION">Todo</option>
                    <option value="IN-PROCESS">In progress</option>
                    <option value="COMPLETED">Done</option>
                  </select>
                  <button type="button" aria-label={`Move ${entry.title} up`} disabled={index === 0} onClick={() => void reorderTask(entry, -1)}>↑</button>
                  <button type="button" aria-label={`Move ${entry.title} down`} disabled={index === items.length - 1} onClick={() => void reorderTask(entry, 1)}>↓</button>
                </article>
              ))}
            </section>
          ))}
        </div>
      ) : (
        <ResourceDataView
          rows={filteredEntries}
          columns={[{ id: 'title', header: 'Task', cell: (entry) => entry.title }]}
          getRowKey={(entry) => entry.id}
          mode="list"
          emptyMessage="No tasks match this view."
          renderItem={(entry) => <EntryRow entry={entry} onEdit={onEdit} onChanged={onChanged} />}
        />
      )}
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
