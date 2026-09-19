import { useEffect, useRef, useState } from 'react';
import { ChevronDown, Filter } from 'lucide-react';

import { Tabs, TabsList, TabsTrigger } from '../ui/tabs';
import { entries, formatDate, formatDateKey, getWeekStart, type Entry } from '../../data';

import { FullMonthCalendar } from '../full-month-calendar';
import { TimeGrid, type TimeGridEvent } from '../time-grid';

function getWeekNumber(date: Date) {
  const thursday = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()));
  thursday.setUTCDate(thursday.getUTCDate() + 4 - (thursday.getUTCDay() || 7));
  const yearStart = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 1));
  return Math.ceil(((thursday.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
}

function addDays(date: Date, amount: number) {
  const result = new Date(date);
  result.setDate(result.getDate() + amount);
  return result;
}

function addMonths(date: Date, amount: number) {
  return new Date(date.getFullYear(), date.getMonth() + amount, 1);
}

function parseMonth(value: string | undefined) {
  if (!value) return null;
  const match = /^(\d{4})-(\d{2})$/.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return null;
  return new Date(year, month - 1, 1);
}

function monthDifference(from: Date, to: Date) {
  return (to.getFullYear() - from.getFullYear()) * 12 + to.getMonth() - from.getMonth();
}

function entryToTimeGrid(entry: Entry): TimeGridEvent {
  const allDay = entry.type === 'task' || Boolean(entry.fields?.all_day);
  const start = allDay
    ? new Date(`${entry.date}T00:00:00`)
    : new Date(String(entry.fields?.start_date ?? `${entry.date}T${entry.time ?? '00:00'}:00`));
  const storedEnd = entry.fields?.end_date;
  const end = storedEnd
    ? new Date(String(storedEnd))
    : new Date(start.getTime() + (allDay ? 24 * 60 * 60 * 1000 : 60 * 60 * 1000));
  return {
    id: entry.id,
    title: entry.title,
    description: entry.description,
    color: entry.color,
    start,
    end,
    allDay,
  };
}

export function CalendarTab({
  onDayClick,
  onEdit,
  initialMonth,
}: {
  onDayClick: () => void;
  onEdit: (entry: Entry) => void;
  initialMonth?: string;
}) {
  const [view, setView] = useState<'day' | 'week' | 'month'>('month');
  const [filterOpen, setFilterOpen] = useState(false);
  const [filter, setFilter] = useState<'all' | 'task' | 'event'>('all');
  const [today] = useState(() => new Date());
  const requestedMonth = parseMonth(initialMonth);
  const requestedMonthOffset = requestedMonth ? monthDifference(today, requestedMonth) : 0;
  const [selectedDate, setSelectedDate] = useState(requestedMonth ?? today);
  const [todayVisible, setTodayVisible] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const pageRefs = useRef<(HTMLDivElement | null)[]>([]);
  const visibleEntries = entries.filter((entry) => filter === 'all' || entry.type === filter);
  const selectedKey = formatDateKey(selectedDate);
  const entriesForDay = (date: Date) => {
    const dayStart = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    const dayEnd = addDays(dayStart, 1);
    return visibleEntries.filter((entry) => {
      const event = entryToTimeGrid(entry);
      return event.end > dayStart && event.start < dayEnd;
    });
  };

  const weekStartsOn = getWeekStart() === 'monday' ? 1 : 0;
  const todayWeekStart = new Date(today);
  todayWeekStart.setDate(today.getDate() - ((today.getDay() - weekStartsOn + 7) % 7));
  const selectedWeekStart = new Date(selectedDate);
  selectedWeekStart.setDate(
    selectedDate.getDate() - ((selectedDate.getDay() - weekStartsOn + 7) % 7),
  );
  const heading =
    view === 'day'
      ? formatDate(selectedKey)
      : view === 'week'
        ? `Week ${getWeekNumber(selectedWeekStart)}`
        : selectedDate.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
  const pageRadius = view === 'day' ? 14 : 6;
  const pageCount = pageRadius * 2 + 1;

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const container = scrollRef.current;
      const targetOffset = view === 'month' ? requestedMonthOffset : 0;
      const targetIndex = Math.max(0, Math.min(pageCount - 1, pageRadius + targetOffset));
      const page = pageRefs.current[targetIndex];
      if (!container || !page) return;
      if (view === 'month') {
        container.scrollTop = page.offsetTop - (container.clientHeight - page.offsetHeight) / 2;
      } else {
        container.scrollLeft = page.offsetLeft - (container.clientWidth - page.offsetWidth) / 2;
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [view, pageRadius, requestedMonthOffset]);

  useEffect(() => {
    const container = scrollRef.current;
    const todayPage = pageRefs.current[pageRadius];
    if (!container || !todayPage) return;

    const observer = new IntersectionObserver(([entry]) => setTodayVisible(entry.isIntersecting), {
      root: container,
      threshold: 0.1,
    });
    observer.observe(todayPage);
    return () => observer.disconnect();
  }, [view, pageRadius]);

  const handleScroll = (event: React.UIEvent<HTMLDivElement>) => {
    const container = event.currentTarget;
    const position =
      view === 'month'
        ? container.scrollTop + container.clientHeight / 2
        : container.scrollLeft + container.clientWidth / 2;
    const pages = Array.from(container.children) as HTMLDivElement[];
    const index = pages.reduce((closest, page, pageIndex) => {
      const pageCenter =
        view === 'month'
          ? page.offsetTop + page.offsetHeight / 2
          : page.offsetLeft + page.offsetWidth / 2;
      const closestCenter =
        view === 'month'
          ? pages[closest].offsetTop + pages[closest].offsetHeight / 2
          : pages[closest].offsetLeft + pages[closest].offsetWidth / 2;
      return Math.abs(pageCenter - position) < Math.abs(closestCenter - position)
        ? pageIndex
        : closest;
    }, 0);
    const offset = index - pageRadius;
    const nextDate =
      view === 'month'
        ? addMonths(today, offset)
        : view === 'week'
          ? addDays(todayWeekStart, offset * 7)
          : addDays(today, offset);
    setSelectedDate(nextDate);
  };

  const jumpToToday = () => {
    setSelectedDate(today);
    setTodayVisible(true);
    requestAnimationFrame(() => {
      const container = scrollRef.current;
      const page = pageRefs.current[pageRadius];
      if (!container || !page) return;
      if (view === 'month') {
        container.scrollTop = page.offsetTop - (container.clientHeight - page.offsetHeight) / 2;
      } else {
        container.scrollLeft = page.offsetLeft - (container.clientWidth - page.offsetWidth) / 2;
      }
    });
  };

  return (
    <div className="calendar-page">
      <header className="page-header">
        <h1>{heading}</h1>
        <Tabs
          value={view}
          onValueChange={(value) => setView(value as typeof view)}
          className="calendar-view-tabs"
        >
          <TabsList className="create-tabs-list">
            <span
              className="create-tabs-track"
              style={{
                width: 'calc((100% - 11px) / 3)',
                transform: `translateX(calc(${view === 'day' ? 0 : view === 'week' ? 1 : 2} * (100% + 3px)))`,
              }}
              aria-hidden="true"
            />
            <TabsTrigger value="day">Day</TabsTrigger>
            <TabsTrigger value="week">Week</TabsTrigger>
            <TabsTrigger value="month">Month</TabsTrigger>
          </TabsList>
        </Tabs>
        <div className="header-actions">
          {!todayVisible && (
            <button
              type="button"
              className="button button-quiet calendar-today-button"
              aria-label="Jump to today"
              onClick={jumpToToday}
            >
              Jump to today
            </button>
          )}
          <div className="filter-wrap">
            <button
              className={`button button-quiet ${filter !== 'all' ? 'selected' : ''}`}
              onClick={() => setFilterOpen(!filterOpen)}
            >
              <Filter size={16} /> Filter <ChevronDown size={15} />
            </button>
            {filterOpen && (
              <div className="popover filter-popover">
                <strong>Show only</strong>
                {(['all', 'task', 'event'] as const).map((value) => (
                  <button
                    key={value}
                    onClick={() => {
                      setFilter(value);
                      setFilterOpen(false);
                    }}
                  >
                    <span className={`radio ${filter === value ? 'checked' : ''}`} />
                    {value === 'all' ? 'Everything' : `${value[0].toUpperCase()}${value.slice(1)}s`}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </header>
      {view === 'day' && (
        <section
          className="calendar-scroll calendar-day-scroll"
          onScroll={handleScroll}
          ref={scrollRef}
        >
          {Array.from({ length: pageCount }, (_, index) => {
            const date = addDays(today, index - pageRadius);
            const dateKey = formatDateKey(date);
            const dateEntries = entriesForDay(date);
            return (
              <div
                className="calendar-day-page calendar-view-panel"
                key={dateKey}
                ref={(element) => {
                  pageRefs.current[index] = element;
                }}
              >
                {dateEntries.length > 0 ? (
                  <TimeGrid
                    className="calendar-day-time-grid"
                    rangeStart={new Date(date.getFullYear(), date.getMonth(), date.getDate())}
                    rangeEnd={addDays(
                      new Date(date.getFullYear(), date.getMonth(), date.getDate()),
                      1,
                    )}
                    events={dateEntries.map(entryToTimeGrid)}
                    now={new Date()}
                    startHour={6}
                    pixelsPerMinute={0.75}
                    renderEvent={(event: TimeGridEvent) => (
                      <button
                        onClick={() => onEdit(dateEntries.find((entry) => entry.id === event.id)!)}
                      >
                        {event.title}
                      </button>
                    )}
                  />
                ) : (
                  <p className="calendar-view-empty">No entries for this day.</p>
                )}
              </div>
            );
          })}
        </section>
      )}
      {view === 'week' && (
        <section
          className="calendar-scroll calendar-week-scroll"
          onScroll={handleScroll}
          ref={scrollRef}
        >
          {Array.from({ length: pageCount }, (_, index) => {
            const pageWeekStart = addDays(todayWeekStart, (index - pageRadius) * 7);

            return (
              <div
                className="calendar-week-page calendar-week-view"
                key={formatDateKey(pageWeekStart)}
                ref={(element) => {
                  pageRefs.current[index] = element;
                }}
              >
                <TimeGrid
                  className="calendar-week-time-grid"
                  rangeStart={pageWeekStart}
                  rangeEnd={addDays(pageWeekStart, 7)}
                  events={visibleEntries.map(entryToTimeGrid)}
                  now={new Date()}
                  startHour={6}
                  pixelsPerMinute={0.6}
                  renderEvent={(event: TimeGridEvent) => (
                    <button
                      onClick={() => onEdit(visibleEntries.find((entry) => entry.id === event.id)!)}
                    >
                      {event.title}
                    </button>
                  )}
                />
              </div>
            );
          })}
        </section>
      )}
      {view === 'month' && (
        <section
          className="calendar-scroll calendar-month-scroll"
          onScroll={handleScroll}
          ref={scrollRef}
        >
          {Array.from({ length: pageCount }, (_, index) => {
            const month = addMonths(today, index - pageRadius);
            return (
              <div
                className="calendar-month-page calendar-view-month"
                key={`${month.getFullYear()}-${month.getMonth()}`}
                ref={(element) => {
                  pageRefs.current[index] = element;
                }}
              >
                <FullMonthCalendar
                  year={month.getFullYear()}
                  month={month.getMonth()}
                  entriesToShow={visibleEntries}
                  onDayClick={onDayClick}
                />
              </div>
            );
          })}
        </section>
      )}
    </div>
  );
}
