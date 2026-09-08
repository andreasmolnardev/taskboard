import { useState } from 'react';
import { ChevronDown, Filter } from 'lucide-react';

import { Tabs, TabsList, TabsTrigger } from '../ui/tabs';
import { entries, formatDate, formatDateKey, getWeekStart } from '../../data';
import { EntryRow } from '../entry-row';
import { FullMonthCalendar } from '../full-month-calendar';

export function CalendarTab({ onDayClick }: { onDayClick: () => void }) {
  const [view, setView] = useState<'day' | 'week' | 'month'>('month');
  const [filterOpen, setFilterOpen] = useState(false);
  const [filter, setFilter] = useState<'all' | 'task' | 'event'>('all');
  const visibleEntries = entries.filter((entry) => filter === 'all' || entry.type === filter);
  const today = new Date();
  const todayKey = formatDateKey(today);
  const dayEntries = visibleEntries.filter((entry) => entry.date === todayKey);
  const weekStartsOn = getWeekStart() === 'monday' ? 1 : 0;
  const weekStart = new Date(today);
  weekStart.setDate(today.getDate() - ((today.getDay() - weekStartsOn + 7) % 7));
  const weekDays = Array.from({ length: 7 }, (_, index) => {
    const date = new Date(weekStart);
    date.setDate(weekStart.getDate() + index);
    return date;
  });

  return (
    <div className="calendar-page">
      <header className="page-header">
        <h1>Calendar</h1>
        <div className="header-actions">
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
          <Tabs value={view} onValueChange={(value) => setView(value as typeof view)}>
            <TabsList className="calendar-view-tabs">
              <TabsTrigger value="day">Day</TabsTrigger>
              <TabsTrigger value="week">Week</TabsTrigger>
              <TabsTrigger value="month">Month</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </header>
      {view === 'day' && (
        <section className="calendar-view-panel">
          <h2>{formatDate(todayKey)}</h2>
          {dayEntries.length > 0 ? (
            dayEntries.map((entry) => <EntryRow entry={entry} key={entry.id} />)
          ) : (
            <p className="calendar-view-empty">No entries for today.</p>
          )}
        </section>
      )}
      {view === 'week' && (
        <section className="calendar-week-view">
          {weekDays.map((date) => {
            const dateKey = formatDateKey(date);
            const dateEntries = visibleEntries.filter((entry) => entry.date === dateKey);
            return (
              <div className="calendar-week-day" key={dateKey}>
                <strong>{date.toLocaleDateString('en-US', { weekday: 'short' })}</strong>
                <span>{date.getDate()}</span>
                {dateEntries.map((entry) => (
                  <small key={entry.id} style={{ borderLeftColor: entry.color }}>
                    {entry.title}
                  </small>
                ))}
              </div>
            );
          })}
        </section>
      )}
      {view === 'month' && (
        <section className="calendar-view-month" key={getWeekStart()}>
          <FullMonthCalendar
            year={today.getFullYear()}
            month={today.getMonth()}
            entriesToShow={visibleEntries}
            onDayClick={onDayClick}
          />
        </section>
      )}
    </div>
  );
}
