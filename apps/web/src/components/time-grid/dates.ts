const DAY_IN_MS = 24 * 60 * 60 * 1000;

export function startOfLocalDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

export function addLocalDays(date: Date, count: number): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + count);
}

export function localDayKey(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function localDayIndex(date: Date, rangeStart: Date): number {
  const dateUtc = Date.UTC(date.getFullYear(), date.getMonth(), date.getDate());
  const startUtc = Date.UTC(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate());
  return Math.round((dateUtc - startUtc) / DAY_IN_MS);
}

export function eachLocalDay(start: Date, end: Date): Date[] {
  const days: Date[] = [];
  for (let day = startOfLocalDay(start); day < end; day = addLocalDays(day, 1)) {
    days.push(day);
  }
  return days;
}
