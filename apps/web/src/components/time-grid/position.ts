import { localDayIndex, startOfLocalDay } from './dates';
import type { CurrentTimePosition, MinutePosition } from './types';

export function minuteOfDay(date: Date): number {
  return date.getHours() * 60 + date.getMinutes() + date.getSeconds() / 60;
}

export function positionInMinutes(
  start: Date,
  end: Date,
  day: Date,
  visibleStartMinute = 0,
  visibleEndMinute = 24 * 60,
): MinutePosition | null {
  if (visibleEndMinute <= visibleStartMinute) {
    throw new RangeError('Visible end minute must be after visible start minute.');
  }

  const dayStart = startOfLocalDay(day);
  const nextDay = new Date(dayStart.getFullYear(), dayStart.getMonth(), dayStart.getDate() + 1);
  const clippedStart = start > dayStart ? start : dayStart;
  const clippedEnd = end < nextDay ? end : nextDay;
  if (clippedEnd <= clippedStart) return null;

  const startMinute = Math.max(visibleStartMinute, minuteOfDay(clippedStart));
  const rawEndMinute = clippedEnd >= nextDay ? 24 * 60 : minuteOfDay(clippedEnd);
  const endMinute = Math.min(visibleEndMinute, rawEndMinute);
  if (endMinute <= startMinute) return null;

  return { top: startMinute - visibleStartMinute, height: endMinute - startMinute };
}

export function getCurrentTimePosition(
  now: Date,
  rangeStart: Date,
  rangeEnd: Date,
  visibleStartMinute = 0,
  visibleEndMinute = 24 * 60,
): CurrentTimePosition | null {
  const dayIndex = localDayIndex(now, startOfLocalDay(rangeStart));
  if (now < startOfLocalDay(rangeStart) || now >= startOfLocalDay(rangeEnd)) return null;
  if (visibleEndMinute <= visibleStartMinute) {
    throw new RangeError('Visible end minute must be after visible start minute.');
  }

  const minute = minuteOfDay(now);
  return {
    dayIndex,
    minute,
    top: minute - visibleStartMinute,
    visible: minute >= visibleStartMinute && minute < visibleEndMinute,
  };
}
