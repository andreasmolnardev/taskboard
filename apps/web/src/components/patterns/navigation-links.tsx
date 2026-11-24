import type { ComponentType } from 'react';
import { Button } from '@/components/ui/button';

export type NavigationStyle = 'sidebar' | 'topbar' | 'rail' | 'bottom';

export function NavigationLinks({
  items,
  activeId,
  layout,
  onNavigate,
  label,
}: {
  items: Array<{ id: string; label: string; icon: ComponentType<{ className?: string }> }>;
  activeId: string;
  layout: NavigationStyle;
  onNavigate: (id: string) => void;
  label: string;
}) {
  const horizontal = layout === 'topbar' || layout === 'bottom';
  return (
    <nav aria-label={label} className={horizontal ? 'flex items-center gap-1' : 'grid gap-1'}>
      {items.map(({ id, label: itemLabel, icon: Icon }) => (
        <Button
          key={id}
          variant={activeId === id ? 'secondary' : 'ghost'}
          size={layout === 'topbar' ? 'sm' : 'default'}
          className={
            layout === 'bottom'
              ? 'min-w-0 flex-1 flex-col gap-1 py-2 text-[10px]'
              : layout === 'rail'
                ? 'justify-center px-0'
                : horizontal
                  ? 'gap-2'
                  : 'justify-start gap-3 font-medium'
          }
          aria-current={activeId === id ? 'page' : undefined}
          aria-label={itemLabel}
          title={itemLabel}
          onClick={() => onNavigate(id)}
        >
          <Icon className="size-4" />
          <span className={layout === 'rail' ? 'sr-only' : ''}>{itemLabel}</span>
        </Button>
      ))}
    </nav>
  );
}
