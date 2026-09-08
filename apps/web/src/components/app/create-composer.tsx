import { useLayoutEffect, useRef, useState } from 'react';
import { Clock3, Plus, X } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../ui/tabs';
import { pb } from '../../api/pocketbase';
import { entries } from '../../data';
import { CreateFields } from '../create-fields';

type Props = {
  open: boolean;
  listOnly: boolean;
  taskEventOnly: boolean;
  type: string;
  color: string;
  onTypeChange: (type: string) => void;
  onColorChange: (color: string) => void;
  onClose: () => void;
  onCreated: () => void;
};
export function CreateComposer({
  open,
  listOnly,
  taskEventOnly,
  type,
  color,
  onTypeChange,
  onColorChange,
  onClose,
  onCreated,
}: Props) {
  const [closing, setClosing] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [formError, setFormError] = useState('');
  const composerRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState<number>();
  useLayoutEffect(() => {
    if (!open || !composerRef.current) return;
    const update = () => setHeight(composerRef.current?.scrollHeight);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(composerRef.current);
    return () => observer.disconnect();
  }, [open, type]);
  if (!open && !closing) return null;
  const tabs = listOnly
    ? ['list', 'calendar']
    : taskEventOnly
      ? ['task', 'event']
      : ['task', 'event', 'list', 'calendar'];
  const tabIndex = tabs.indexOf(type);
  const close = () => {
    if (closing) return;
    setClosing(true);
  };
  const finishClose = (event: React.AnimationEvent<HTMLDivElement>) => {
    if (event.target === event.currentTarget && closing) {
      setClosing(false);
      onClose();
    }
  };
  const save = async () => {
    if (!title.trim()) return;
    try {
      const owner = pb.authStore.record?.id;
      const today = new Date();
      const date = today.toISOString();
      const collection = type === 'task' ? 'todos' : `${type}s`;
      const record = await pb.collection(collection).create(
        type === 'task'
          ? { owner, title, description, completed: false, due_date: date }
          : type === 'event'
            ? { owner, title, description, start_date: date, all_day: true }
            : { owner, name: title, description, color },
      );
      if (type === 'task' || type === 'event') {
        entries.push({
          id: record.id,
          title,
          description,
          date: date.slice(0, 10),
          list: type === 'task' ? 'Tasks' : 'Calendar',
          color,
          type: type as 'task' | 'event',
        });
      }
      setTitle('');
      setDescription('');
      setFormError('');
      onCreated();
      close();
    } catch {
      setFormError('Could not create this task.');
    }
  };
  return (
    <div
      className={`modal-backdrop ${closing ? 'is-closing' : ''}`}
      onMouseDown={close}
      onAnimationEnd={finishClose}
    >
      <div
        ref={composerRef}
        className="composer create-modal"
        style={{ height: height ? `${height}px` : undefined }}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="composer-header">
          <h2>Create new</h2>
          <button className="icon-button" onClick={close} aria-label="Close">
            <X size={19} />
          </button>
        </div>
        <Tabs
          value={type}
          onValueChange={onTypeChange}
          className={`create-tabs ${listOnly ? 'create-tabs-list-only' : ''} ${taskEventOnly ? 'create-tabs-task-event-only' : ''}`}
        >
          <TabsList className="create-tabs-list">
            <span
              className="create-tabs-track"
              style={{ transform: `translateX(calc(${tabIndex} * (100% + 3px)))` }}
              aria-hidden="true"
            />
            {!listOnly && <TabsTrigger value="task">Task</TabsTrigger>}
            {!listOnly && <TabsTrigger value="event">Event</TabsTrigger>}
            {!taskEventOnly && <TabsTrigger value="list">List</TabsTrigger>}
            {!taskEventOnly && <TabsTrigger value="calendar">Calendar</TabsTrigger>}
          </TabsList>
          <TabsContent value="task">
            <CreateFields
              title={title}
              setTitle={setTitle}
              description={description}
              setDescription={setDescription}
              placeholder="What needs doing?"
            />
          </TabsContent>
          <TabsContent value="event">
            <CreateFields
              title={title}
              setTitle={setTitle}
              description={description}
              setDescription={setDescription}
              placeholder="What is happening?"
            />
            <div className="composer-meta">
              <button>
                <Clock3 size={16} /> Today
              </button>
              <button>
                <Clock3 size={16} /> Add time
              </button>
            </div>
          </TabsContent>
          <TabsContent value="list">
            <CreateFields
              title={title}
              setTitle={setTitle}
              description={description}
              setDescription={setDescription}
              placeholder="Name this list"
              color={color}
              setColor={onColorChange}
              showColor
            />
          </TabsContent>
          <TabsContent value="calendar">
            <CreateFields
              title={title}
              setTitle={setTitle}
              description={description}
              setDescription={setDescription}
              placeholder="Name this calendar"
              color={color}
              setColor={onColorChange}
              showColor
            />
          </TabsContent>
        </Tabs>
        {formError && <p className="form-error">{formError}</p>}
        <button className="button button-primary composer-save" onClick={() => void save()}>
          <Plus size={17} /> Create {type}
        </button>
      </div>
    </div>
  );
}
