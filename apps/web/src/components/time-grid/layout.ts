import type { EventColumn, TimeGridEvent } from './types';

function validateEvent(event: TimeGridEvent): void {
  if (
    Number.isNaN(event.start.getTime()) ||
    Number.isNaN(event.end.getTime()) ||
    event.end <= event.start
  ) {
    throw new RangeError(`Event "${event.id}" must have a valid end after its start.`);
  }
}

/** Assigns the lowest free column to each event. Touching edges do not overlap. */
export function layoutOverlappingEvents(events: readonly TimeGridEvent[]): EventColumn[] {
  const sorted = [...events].sort(
    (left, right) =>
      left.start.getTime() - right.start.getTime() ||
      left.end.getTime() - right.end.getTime() ||
      left.id.localeCompare(right.id),
  );
  sorted.forEach(validateEvent);

  const result: EventColumn[] = [];
  let group: EventColumn[] = [];
  let groupEnd = Number.NEGATIVE_INFINITY;
  let columnEnds: number[] = [];

  const finishGroup = () => {
    const columnCount = columnEnds.length;
    for (const item of group) item.columnCount = columnCount;
    group = [];
    columnEnds = [];
  };

  for (const event of sorted) {
    const start = event.start.getTime();
    if (group.length > 0 && start >= groupEnd) finishGroup();

    const freeColumn = columnEnds.findIndex((end) => end <= start);
    const column = freeColumn === -1 ? columnEnds.length : freeColumn;
    columnEnds[column] = event.end.getTime();
    groupEnd = Math.max(groupEnd, event.end.getTime());

    const item = { event, column, columnCount: 0 };
    group.push(item);
    result.push(item);
  }

  if (group.length > 0) finishGroup();
  return result;
}
