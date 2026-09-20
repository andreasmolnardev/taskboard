import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Plus, X } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../ui/tabs';
import { apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';
import { entries, getActiveContainers, lists, type Entry } from '../../data';
import { CreateFields } from '../create-fields';
import { localToRecord, recordToLocal } from './create-composer.helpers';

type Props = {
  open: boolean;
  listOnly: boolean;
  taskEventOnly: boolean;
  type: string;
  color: string;
  editing?: Entry | null;
  onTypeChange: (type: string) => void;
  onColorChange: (color: string) => void;
  onClose: () => void;
  onCreated: () => void;
};
type Form = Record<string, string | boolean>;

function repeatFromRule(value: unknown) {
  const rule = String(value ?? '').toUpperCase();
  if (!rule) return 'none';
  if (rule.includes('FREQ=DAILY') && rule.includes('BYDAY=MO,TU,WE,TH,FR')) return 'weekdays';
  if (rule.includes('FREQ=DAILY')) return 'daily';
  if (rule.includes('FREQ=WEEKLY')) return 'weekly';
  if (rule.includes('FREQ=MONTHLY')) return 'monthly';
  if (rule.includes('FREQ=YEARLY')) return 'yearly';
  return 'custom';
}

function repeatRule(value: string, original: unknown) {
  switch (value) {
    case 'daily':
      return 'FREQ=DAILY';
    case 'weekdays':
      return 'FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR';
    case 'weekly':
      return 'FREQ=WEEKLY';
    case 'monthly':
      return 'FREQ=MONTHLY';
    case 'yearly':
      return 'FREQ=YEARLY';
    case 'custom':
      return String(original ?? '');
    default:
      return '';
  }
}

const textFields = (type: string) =>
  type === 'task'
    ? [
        ['status', 'Status (NEEDS-ACTION, IN-PROCESS, COMPLETED, CANCELLED)'],
        ['priority', 'Priority (0–9)'],
        ['percent_complete', 'Percent complete'],
        ['duration', 'Duration'],
        ['categories', 'Categories'],
        ['exdate', 'Excluded dates'],
        ['organizer', 'Organizer'],
        ['attendees', 'Attendees'],
        ['url', 'URL'],
        ['class', 'Visibility (PUBLIC, PRIVATE, CONFIDENTIAL)'],
        ['geo', 'Coordinates'],
      ]
    : [
        ['status', 'Status (TENTATIVE, CONFIRMED, CANCELLED)'],
        ['location', 'Location'],
        ['duration', 'Duration'],
        ['categories', 'Categories'],
        ['exdate', 'Excluded dates'],
        ['organizer', 'Organizer'],
        ['attendees', 'Attendees'],
        ['url', 'URL'],
        ['class', 'Visibility (PUBLIC, PRIVATE, CONFIDENTIAL)'],
        ['geo', 'Coordinates'],
      ];

export function CreateComposer({
  open,
  listOnly,
  taskEventOnly,
  type,
  color,
  editing,
  onTypeChange,
  onColorChange,
  onClose,
  onCreated,
}: Props) {
  const [form, setForm] = useState<Form>({});
  const composerRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState<number>();
  const [closing, setClosing] = useState(false);
  const set = (key: string, value: string | boolean) =>
    setForm((current) => ({ ...current, [key]: value }));

  useEffect(() => {
    if (!open) return;
    setClosing(false);
    const fields = (editing?.fields ?? {}) as Form;
    const entryType = editing?.type ?? type;
    const defaultContainer = getActiveContainers(lists).find(
      (container) => container.kind === (entryType === 'task' ? 'list' : 'calendar'),
    );
    setForm({
      title: editing?.title ?? '',
      description: editing?.description ?? '',
      ...fields,
      reminder_minutes: fields.reminder_minutes ?? '',
      repeat: fields.repeat ?? repeatFromRule(fields.rrule),
      recurrence_scope: editing?.masterId ? 'occurrence' : 'series',
      ...(entryType === 'task'
        ? { list: fields.list ?? editing?.containerId ?? defaultContainer?.id ?? '' }
        : {
            calendar: fields.calendar ?? editing?.containerId ?? defaultContainer?.id ?? '',
            all_day: fields.all_day ?? true,
          }),
    });
  }, [editing, open]);
  useLayoutEffect(() => {
    if (!open || !composerRef.current) return;
    const update = () => setHeight(composerRef.current?.scrollHeight);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(composerRef.current);
    return () => observer.disconnect();
  }, [open, type, form]);
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
    if (!String(form.title ?? '').trim()) return;
    const owner = pb.authStore.record?.id;
    const now = new Date().toISOString();
    const collection =
      type === 'task'
        ? 'todos'
        : type === 'event'
          ? 'events'
          : type === 'calendar'
            ? 'calendars'
            : 'lists';
    const { reminder_minutes: reminderMinutes, ...entryForm } = form;
    delete entryForm.error;
    delete entryForm.recurrence_scope;
    delete entryForm.recurrence_id;
    delete entryForm.repeat;
    const allDay = type === 'event' && Boolean(form.all_day);
    const startInput = String(form.start_local ?? form.start_date ?? '').trim();
    const endInput = String(form.end_local ?? form.end_date ?? '').trim();
    const dueInput = String(form.due_local ?? form.due_date ?? '').trim();
    const start = localToRecord(startInput, allDay);
    const end = localToRecord(endInput, allDay);
    const due = localToRecord(dueInput, false);
    if ((type === 'event' && startInput && !start.index) || (type === 'task' && dueInput && !due.index)) {
      set('error', 'Enter a valid date and time.');
      return;
    }
    if (type === 'event' && endInput && !end.index) {
      set('error', 'Enter a valid end date and time.');
      return;
    }
    if (type === 'event' && start.index && end.index && end.index <= start.index) {
      set('error', 'End must be after start.');
      return;
    }
    const values =
      type === 'list' || type === 'calendar'
        ? {
            owner,
            name: String(form.title).trim(),
            description: String(form.description ?? ''),
            color,
          }
        : {
            ...entryForm,
            rrule: repeatRule(String(form.repeat ?? 'none'), form.rrule),
            owner,
            title: String(form.title).trim(),
            description: String(form.description ?? ''),
            uid: String(form.uid ?? crypto.randomUUID()),
            dtstamp: now,
            ...(type === 'task'
              ? {
                  due_date: due.index || now,
                  due_local: due.local,
                  time_mode: due.mode,
                  timezone:
                    due.mode === 'zoned' ? Intl.DateTimeFormat().resolvedOptions().timeZone : '',
                  completed: form.status === 'COMPLETED',
                }
              : {
                  start_date: start.index || now,
                  start_local: start.local,
                  end_date: end.index || '',
                  end_local: end.local,
                  time_mode: start.mode,
                  timezone:
                    start.mode === 'zoned' ? Intl.DateTimeFormat().resolvedOptions().timeZone : '',
                }),
          };
    const saveValues = { ...values } as Record<string, unknown>;
    if (editing?.masterId && type === 'event' && form.recurrence_scope === 'series') {
      for (const field of [
        'start_date',
        'start_local',
        'end_date',
        'end_local',
        'all_day',
        'timezone',
        'time_mode',
      ])
        delete saveValues[field];
    }
    try {
      if (editing?.masterId && type === 'event' && form.recurrence_scope === 'occurrence') {
        const recurrenceId = String(
          form.recurrence_id ?? editing.fields?.recurrence_id ?? editing.id.split(':')[1] ?? '',
        );
        const response = await apiFetch(`/api/calendar/occurrences/${editing.masterId}/override`, {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${pb.authStore.token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ recurrenceId, values: saveValues }),
        });
        if (!response.ok) throw new Error('Could not save this occurrence.');
        onCreated();
        close();
        return;
      }
      const record = editing
        ? await pb.collection(collection).update(editing.masterId ?? editing.id, saveValues)
        : await pb.collection(collection).create(saveValues);
      if (type === 'list' || type === 'calendar') {
        onCreated();
        close();
        return;
      }
      if (String(reminderMinutes) && Number.isFinite(Number(reminderMinutes))) {
        const targetAt = type === 'task' ? record.due_date : record.start_date;
        await apiFetch('/api/reminders', {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${pb.authStore.token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            targetKind: type,
            targetId: record.id,
            at: new Date(String(targetAt)).toISOString(),
            offsetMinutes: Number(reminderMinutes),
            channel: 'in-app',
          }),
        });
      }
      const containerId = String(type === 'task' ? record.list : record.calendar);
      const container = lists.find((item) => item.id === containerId);
      const entry: Entry = {
        id: record.id,
        title: String(record.title),
        description: String(record.description ?? ''),
        date: String(record.due_date ?? record.start_date ?? now).slice(0, 10),
        time: record.all_day ? undefined : String(record.start_date ?? '').slice(11, 16),
        containerId,
        list: container?.name ?? (type === 'task' ? 'Tasks' : 'Calendar'),
        color: container?.color ?? color,
        type: type as 'task' | 'event',
        done: Boolean(record.completed || record.status === 'COMPLETED'),
        fields: record,
      };
      const index = entries.findIndex((item) => item.id === entry.id);
      if (index >= 0) entries[index] = entry;
      else entries.push(entry);
      onCreated();
      close();
    } catch {
      set('error', 'Could not save this entry.');
    }
  };
  const renderExtraFields = (entryType: string) => (
    <div className="ical-fields">

      {editing?.masterId && entryType === 'event' && (
        <label>
          Edit scope
          <select
            value={String(form.recurrence_scope ?? 'occurrence')}
            onChange={(e) => set('recurrence_scope', e.target.value)}
          >
            <option value="occurrence">This occurrence</option>
            <option value="series">Whole series</option>
          </select>
        </label>
      )}
      <label>
        Created / updated
        <input
          type="datetime-local"
          value={String(form.dtstamp ?? '').slice(0, 16)}
          onChange={(e) =>
            set('dtstamp', e.target.value ? new Date(e.target.value).toISOString() : '')
          }
        />
      </label>
      {entryType === 'event' && (
        <label>
          Start
          <input
            type={form.all_day === true ? 'date' : 'datetime-local'}
            value={recordToLocal(form.start_local ?? form.start_date, form.all_day === true)}
            onChange={(e) => set('start_local', e.target.value)}
          />
        </label>
      )}
      {entryType === 'event' && (
        <label>
          End
          <input
            type={form.all_day === true ? 'date' : 'datetime-local'}
            value={recordToLocal(form.end_local ?? form.end_date, form.all_day === true)}
            onChange={(e) => set('end_local', e.target.value)}
          />
        </label>
      )}
      {entryType === 'task' && (
        <label>
          Due
          <input
            type="datetime-local"
            value={recordToLocal(form.due_local ?? form.due_date, false)}
            onChange={(e) => set('due_local', e.target.value)}
          />
        </label>
      )}

      <label>
        {entryType === 'task' ? 'List' : 'Calendar'}
        <select
          value={String(form[entryType === 'task' ? 'list' : 'calendar'] ?? '')}
          onChange={(event) => set(entryType === 'task' ? 'list' : 'calendar', event.target.value)}
        >
          {getActiveContainers(lists)
            .filter((container) => container.kind === (entryType === 'task' ? 'list' : 'calendar'))
            .map((container) => (
              <option key={container.id} value={container.id}>
                {container.name}
              </option>
            ))}
        </select>
      </label>
      {entryType === 'event' && (
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={Boolean(form.all_day)}
            onChange={(e) => set('all_day', e.target.checked)}
          />{' '}
          All day
        </label>
      )}
      <label>
        Repeat
        <select
          value={String(form.repeat ?? 'none')}
          onChange={(e) => set('repeat', e.target.value)}
        >
          <option value="none">Does not repeat</option>
          <option value="daily">Every day</option>
          <option value="weekdays">Weekdays</option>
          <option value="weekly">Every week</option>
          <option value="monthly">Every month</option>
          <option value="yearly">Every year</option>
          <option value="custom">Imported custom rule</option>
        </select>
      </label>
      <label>
        Reminder
        <select
          value={String(form.reminder_minutes ?? '')}
          onChange={(e) => set('reminder_minutes', e.target.value)}
        >
          <option value="">No reminder</option>
          <option value="-15">15 minutes before</option>
          <option value="-30">30 minutes before</option>
          <option value="-60">1 hour before</option>
          <option value="0">At start</option>
        </select>
      </label>
      {textFields(entryType).map(([key, label]) => (
        <label key={key}>
          {label}
          <input value={String(form[key] ?? '')} onChange={(e) => set(key, e.target.value)} />
        </label>
      ))}
    </div>
  );
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
          <h2>{editing ? 'Edit' : 'Create new'}</h2>
          <button type="button" className="icon-button" onClick={close} aria-label="Close">
            <X size={19} />
          </button>
        </div>
        <Tabs
          value={type}
          onValueChange={editing ? undefined : onTypeChange}
          className={`create-tabs ${listOnly ? 'create-tabs-list-only' : ''} ${taskEventOnly ? 'create-tabs-task-event-only' : ''}`}
        >
          <TabsList className="create-tabs-list">
            <span
              className="create-tabs-track"
              style={{ transform: `translateX(calc(${tabIndex} * (100% + 3px)))` }}
              aria-hidden="true"
            />
            {!listOnly && (
              <TabsTrigger value="task" disabled={Boolean(editing)}>
                Task
              </TabsTrigger>
            )}
            {!listOnly && (
              <TabsTrigger value="event" disabled={Boolean(editing)}>
                Event
              </TabsTrigger>
            )}
            {!taskEventOnly && (
              <TabsTrigger value="list" disabled={Boolean(editing)}>
                List
              </TabsTrigger>
            )}
            {!taskEventOnly && (
              <TabsTrigger value="calendar" disabled={Boolean(editing)}>
                Calendar
              </TabsTrigger>
            )}
          </TabsList>
          {(['task', 'event'] as const).map((entryType) => (
            <TabsContent value={entryType} key={entryType}>
              <CreateFields
                title={String(form.title ?? '')}
                setTitle={(value) => set('title', value)}
                description={String(form.description ?? '')}
                setDescription={(value) => set('description', value)}
                placeholder={entryType === 'task' ? 'What needs doing?' : 'What is happening?'}
              />
              <details className="advanced-options">
                <summary>Advanced options</summary>
                {renderExtraFields(entryType)}
              </details>
            </TabsContent>
          ))}
          <TabsContent value="list">
            <CreateFields
              title={String(form.title ?? '')}
              setTitle={(value) => set('title', value)}
              description={String(form.description ?? '')}
              setDescription={(value) => set('description', value)}
              placeholder="Name this list"
              color={color}
              setColor={onColorChange}
              showColor
            />
          </TabsContent>
          <TabsContent value="calendar">
            <CreateFields
              title={String(form.title ?? '')}
              setTitle={(value) => set('title', value)}
              description={String(form.description ?? '')}
              setDescription={(value) => set('description', value)}
              placeholder="Name this calendar"
              color={color}
              setColor={onColorChange}
              showColor
            />
          </TabsContent>
        </Tabs>
        {form.error && <p className="form-error">{String(form.error)}</p>}
        <button className="button button-primary composer-save" onClick={() => void save()}>
          <Plus size={17} /> {editing ? 'Save changes' : `Create ${type}`}
        </button>
      </div>
    </div>
  );
}
