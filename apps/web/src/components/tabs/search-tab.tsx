import { useState } from 'react';
import { CalendarDays, Check } from 'lucide-react';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../ui/command';
import { entries } from '../../data';

export function SearchTab({
  open,
  onOpenChange,
  className,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  className?: string;
}) {
  const [query, setQuery] = useState('');
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      className={className}
      title="Search Taskboard"
      description="Search your tasks and events"
    >
      <CommandInput
        autoFocus
        value={query}
        onValueChange={setQuery}
        placeholder="Search your entries"
      />
      <CommandList>
        <CommandEmpty>No entries found.</CommandEmpty>
        <CommandGroup heading="Entries">
          {entries.map((entry) => (
            <CommandItem key={entry.id} value={`${entry.title} ${entry.description} ${entry.list}`}>
              <span className="command-entry-icon" style={{ color: entry.color }}>
                {entry.type === 'event' ? <CalendarDays size={16} /> : <Check size={16} />}
              </span>
              <span className="command-entry-copy">
                <strong>{entry.title}</strong>
                <small>{entry.description || entry.list}</small>
              </span>
              <span className="command-entry-list">{entry.list}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
