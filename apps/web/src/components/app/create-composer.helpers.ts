export function localToRecord(value: string, allDay: boolean) {
  if (!value) return { index: '', local: '', mode: allDay ? 'date' : 'floating' };

  const compactDate = value.match(/^(\d{4})(\d{2})(\d{2})$/);
  const compactDateTime = value.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/);
  const normalizedValue = compactDateTime
    ? `${compactDateTime[1]}-${compactDateTime[2]}-${compactDateTime[3]}T${compactDateTime[4]}:${compactDateTime[5]}`
    : compactDate
      ? `${compactDate[1]}-${compactDate[2]}-${compactDate[3]}`
      : value;
  const datePart = normalizedValue.slice(0, 10);
  if (allDay) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(datePart))
      return { index: '', local: '', mode: 'date' };
    return {
      index: `${datePart}T00:00:00.000Z`,
      local: datePart.replaceAll('-', ''),
      mode: 'date',
    };
  }

  const localValue = /^\d{4}-\d{2}-\d{2}$/.test(normalizedValue)
    ? `${normalizedValue}T00:00`
    : normalizedValue;
  const date = new Date(localValue);
  if (Number.isNaN(date.getTime())) return { index: '', local: '', mode: 'floating' };
  const match = localValue.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
  if (!match) return { index: '', local: '', mode: 'floating' };
  return {
    index: date.toISOString(),
    local: `${match[1]}${match[2]}${match[3]}T${match[4]}${match[5]}00`,
    mode: 'zoned',
  };
}

export function recordToLocal(value: unknown, allDay: boolean) {
  const raw = String(value ?? '').trim();

  if (!raw) return '';

  const compactDate = /^(\d{4})(\d{2})(\d{2})$/;
  const compactDateTime = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/;
  const dateTime = /^(\d{4}-\d{2}-\d{2})(?:(?:T| )(\d{2}):(\d{2}))?/;
  const compactMatch = raw.match(compactDate);
  const compactDateTimeMatch = raw.match(compactDateTime);

  if (compactDateTimeMatch) {
    const date = formatDateParts(
      compactDateTimeMatch[1],
      compactDateTimeMatch[2],
      compactDateTimeMatch[3],
    );
    const hour = Number(compactDateTimeMatch[4]);
    const minute = Number(compactDateTimeMatch[5]);
    if (!date || hour > 23 || minute > 59) return '';
    return allDay ? date : `${date}T${compactDateTimeMatch[4]}:${compactDateTimeMatch[5]}`;
  }
  if (compactMatch) {
    const date = formatDateParts(compactMatch[1], compactMatch[2], compactMatch[3]);
    return date ? (allDay ? date : `${date}T00:00`) : '';
  }

  const match = raw.match(dateTime);
  if (!match) return '';
  const [year, month, day] = match[1].split('-');
  const date = formatDateParts(year, month, day);
  if (!date) return '';
  const hour = Number(match[2] ?? 0);
  const minute = Number(match[3] ?? 0);
  if (hour > 23 || minute > 59) return '';
  return allDay ? date : `${date}T${match[2] ?? '00'}:${match[3] ?? '00'}`;
}

function formatDateParts(year: string, month: string, day: string) {
  const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)));
  if (
    date.getUTCFullYear() !== Number(year) ||
    date.getUTCMonth() !== Number(month) - 1 ||
    date.getUTCDate() !== Number(day)
  )
    return '';
  return `${year}-${month}-${day}`;
}
