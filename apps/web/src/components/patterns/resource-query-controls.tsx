import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import type { ResourceQuery } from '@/lib/slopstack/resource-client';
import type { QueryDefinition, ResourceDefinition } from '@/lib/slopstack/types';

type QueryResource = Pick<ResourceDefinition, 'fields' | 'label' | 'labelPlural'> & { query?: QueryDefinition };

/** Search, sort, and filter controls derived from a resource's declarative query contract. */
export function ResourceQueryControls({
  resource,
  value,
  onChange,
  total,
}: {
  resource: QueryResource;
  value: ResourceQuery;
  onChange: (next: ResourceQuery) => void;
  total?: number;
}) {
  const query = resource.query ?? {};
  const filters = query.filters ?? [];
  const fieldMap = new Map(resource.fields.map((field) => [field.name, field]));
  const currentPage = value.page ?? 1;
  const pageSize = value.pageSize ?? query.pageSize ?? 25;
  const pageCount = total === undefined ? undefined : Math.max(1, Math.ceil(total / pageSize));
  const patch = (next: Partial<ResourceQuery>) => onChange({ ...value, ...next, page: next.page ?? 1 });

  return (
    <section className="grid gap-3" aria-label="Resource query controls">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {query.searchFields?.length ? (
          <div className="grid gap-1.5 sm:col-span-2">
            <Label htmlFor="resource-query-search">Search</Label>
            <Input id="resource-query-search" type="search" value={value.search ?? ''} placeholder={`Search ${resourceLabel(resource)}…`} onChange={(event) => patch({ search: event.target.value })} />
          </div>
        ) : null}
        {query.sortOptions?.length ? (
          <div className="grid gap-1.5">
            <Label htmlFor="resource-query-sort">Sort by</Label>
            <Select value={value.sort ?? query.defaultSort ?? ''} onValueChange={(sort) => patch({ sort })}>
              <SelectTrigger id="resource-query-sort"><SelectValue placeholder="Choose sort" /></SelectTrigger>
              <SelectContent>{query.sortOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent>
            </Select>
          </div>
        ) : null}
        {filters.map((filter) => {
          const field = fieldMap.get(filter.field);
          const currentValue = String(value.filters?.[filter.id] ?? '');
          const choices = filter.options ?? (field?.kind === 'select' ? field.options : undefined);
          const updateFilter = (selected: string) => {
            const nextFilters = { ...(value.filters ?? {}) };
            if (selected) nextFilters[filter.id] = selected;
            else delete nextFilters[filter.id];
            patch({ filters: nextFilters });
          };
          return (
            <div className="grid gap-1.5" key={filter.id}>
              <Label htmlFor={`resource-filter-${filter.id}`}>{filter.label}</Label>
              {choices?.length ? (
                <Select value={currentValue || '__all'} onValueChange={(selected) => updateFilter(selected === '__all' ? '' : selected)}>
                  <SelectTrigger id={`resource-filter-${filter.id}`}><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all">All</SelectItem>
                    {choices.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              ) : (
                <Input id={`resource-filter-${filter.id}`} type={field?.kind === 'date' ? 'date' : field?.kind === 'number' ? 'number' : 'text'} value={currentValue} onChange={(event) => updateFilter(event.target.value)} placeholder={filter.label} />
              )}
            </div>
          );
        })}
      </div>
      {total !== undefined && (
        <div className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
          <span>{total ? `${Math.min((currentPage - 1) * pageSize + 1, total)}–${Math.min(currentPage * pageSize, total)} of ${total}` : 'No results'}</span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={currentPage <= 1} onClick={() => onChange({ ...value, page: currentPage - 1 })}>Previous</Button>
            <span>Page {currentPage}{pageCount === undefined ? '' : ` of ${pageCount}`}</span>
            <Button variant="outline" size="sm" disabled={pageCount !== undefined && currentPage >= pageCount} onClick={() => onChange({ ...value, page: currentPage + 1 })}>Next</Button>
          </div>
        </div>
      )}
    </section>
  );
}

function resourceLabel(resource: QueryResource) {
  return resource.labelPlural ?? `${resource.label}s`;
}
