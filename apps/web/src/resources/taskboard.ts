import { defineResource } from '../lib/slopstack';

type TaskRecord = Record<string, unknown> & { id: string; title: string };
type EventRecord = Record<string, unknown> & { id: string; title: string };
type ContainerRecord = Record<string, unknown> & { id: string; name: string };

const ownership = {
  list: { authenticated: true },
  get: { authenticated: true },
  create: { authenticated: true },
  update: { authenticated: true },
  delete: { authenticated: true },
} as const;

export const taskResource = defineResource<TaskRecord>({
  id: 'tasks',
  label: 'Task',
  labelPlural: 'Tasks',
  description: 'Tasks stored in Taskboard lists, including their recurrence and completion state.',
  source: { kind: 'pocketbase', collection: 'todos' },
  fields: [
    { name: 'title', label: 'Title', kind: 'text', required: true, maxLength: 240 },
    { name: 'description', label: 'Description', kind: 'text', multiline: true, rows: 3 },
    { name: 'list', label: 'List', kind: 'relation', resource: 'lists', cardinality: 'one' },
    { name: 'due_date', label: 'Due date', kind: 'datetime' },
    {
      name: 'status',
      label: 'Status',
      kind: 'select',
      options: [
        { label: 'Needs action', value: 'NEEDS-ACTION' },
        { label: 'In progress', value: 'IN-PROCESS' },
        { label: 'Completed', value: 'COMPLETED' },
        { label: 'Cancelled', value: 'CANCELLED' },
      ],
    },
    { name: 'rrule', label: 'Recurrence rule', kind: 'text' },
  ],
  operations: { list: true, get: true, create: true, update: true, delete: true },
  permissions: ownership,
  identify: (record) => record.id,
  query: {
    searchFields: ['title', 'description'],
    defaultSort: 'due_date',
    sortOptions: [
      { label: 'Due date', value: 'due_date' },
      { label: 'Title', value: 'title' },
      { label: 'Recently updated', value: '-updated' },
    ],
    filters: [
      {
        id: 'status',
        label: 'Status',
        field: 'status',
        operator: 'equals',
        options: [
          { label: 'Needs action', value: 'NEEDS-ACTION' },
          { label: 'In progress', value: 'IN-PROCESS' },
          { label: 'Completed', value: 'COMPLETED' },
          { label: 'Cancelled', value: 'CANCELLED' },
        ],
      },
    ],
    pageSize: 50,
  },
});

export const eventResource = defineResource<EventRecord>({
  id: 'events',
  label: 'Event',
  labelPlural: 'Events',
  description: 'Calendar events with all-day, floating-time, and recurrence details.',
  source: { kind: 'pocketbase', collection: 'events' },
  fields: [
    { name: 'title', label: 'Title', kind: 'text', required: true, maxLength: 240 },
    { name: 'description', label: 'Description', kind: 'text', multiline: true, rows: 3 },
    {
      name: 'calendar',
      label: 'Calendar',
      kind: 'relation',
      resource: 'calendars',
      cardinality: 'one',
    },
    { name: 'start_date', label: 'Starts', kind: 'datetime' },
    { name: 'end_date', label: 'Ends', kind: 'datetime' },
    { name: 'all_day', label: 'All day', kind: 'boolean' },
    { name: 'location', label: 'Location', kind: 'text' },
    { name: 'rrule', label: 'Recurrence rule', kind: 'text' },
  ],
  operations: { list: true, get: true, create: true, update: true, delete: true },
  permissions: ownership,
  identify: (record) => record.id,
  query: {
    searchFields: ['title', 'description', 'location'],
    defaultSort: 'start_date',
    sortOptions: [
      { label: 'Start time', value: 'start_date' },
      { label: 'Title', value: 'title' },
      { label: 'Recently updated', value: '-updated' },
    ],
    pageSize: 50,
  },
});

function containerResource(id: 'lists' | 'calendars', label: 'List' | 'Calendar') {
  return defineResource<ContainerRecord>({
    id,
    label,
    labelPlural: `${label}s`,
    source: { kind: 'pocketbase', collection: id },
    fields: [
      { name: 'name', label: 'Name', kind: 'text', required: true, maxLength: 120 },
      { name: 'description', label: 'Description', kind: 'text', multiline: true },
      {
        name: 'color',
        label: 'Color',
        kind: 'custom',
        renderer: 'taskboard-color',
        validate: (value) =>
          /^#[0-9a-f]{6}$/i.test(String(value)) ? undefined : 'Choose a valid color.',
      },
      { name: 'archived', label: 'Archived', kind: 'boolean' },
    ],
    operations: { list: true, get: true, create: true, update: true, delete: true },
    permissions: ownership,
    identify: (record) => record.id,
    query: {
      searchFields: ['name', 'description'],
      defaultSort: 'name',
      sortOptions: [{ label: 'Name', value: 'name' }],
    },
  });
}

export const listResource = containerResource('lists', 'List');
export const calendarResource = containerResource('calendars', 'Calendar');
