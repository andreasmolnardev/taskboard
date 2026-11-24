import { useMemo, useState, type ReactNode } from 'react';
import { DataTable, type DataTableColumn } from './data-table';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';

export type ResourceDataViewMode = 'table' | 'cards' | 'list';

export function ResourceDataView<Row>({
  rows,
  columns,
  getRowKey,
  mode = 'table',
  renderItem,
  emptyMessage = 'No items to show.',
  selectable = false,
  selection: controlledSelection,
  onSelectionChange,
  onRowClick,
}: {
  rows: readonly Row[];
  columns: readonly DataTableColumn<Row>[];
  getRowKey: (row: Row) => string;
  mode?: ResourceDataViewMode;
  renderItem?: (row: Row) => ReactNode;
  emptyMessage?: string;
  selectable?: boolean;
  selection?: readonly string[];
  onSelectionChange?: (selection: readonly string[]) => void;
  onRowClick?: (row: Row) => void;
}) {
  const [localSelection, setLocalSelection] = useState<readonly string[]>([]);
  const selection = controlledSelection ?? localSelection;
  const keys = useMemo(() => rows.map(getRowKey), [getRowKey, rows]);
  const selectedVisibleCount = keys.filter((key) => selection.includes(key)).length;
  const allSelected = keys.length > 0 && selectedVisibleCount === keys.length;
  const partlySelected = selectedVisibleCount > 0 && !allSelected;

  const updateSelection = (next: readonly string[]) => {
    setLocalSelection(next);
    onSelectionChange?.(next);
  };
  const toggleRow = (key: string, checked: boolean) => {
    updateSelection(checked ? [...new Set([...selection, key])] : selection.filter((item) => item !== key));
  };
  const toggleVisible = (checked: boolean) => {
    updateSelection(checked ? [...new Set([...selection, ...keys])] : selection.filter((item) => !keys.includes(item)));
  };

  if (!rows.length) return <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">{emptyMessage}</div>;

  const visibleColumns: DataTableColumn<Row>[] = selectable ? [
    {
      id: '__selection',
      header: (
        <Checkbox
          aria-label="Select all visible rows"
          checked={allSelected ? true : partlySelected ? 'indeterminate' : false}
          onCheckedChange={(checked) => toggleVisible(checked === true)}
        />
      ),
      cell: (row) => {
        const key = getRowKey(row);
        return <Checkbox aria-label={`Select row ${key}`} checked={selection.includes(key)} onCheckedChange={(checked) => toggleRow(key, checked === true)} />;
      },
    },
    ...columns,
  ] : [...columns];

  if (mode === 'table') {
    return <DataTable columns={visibleColumns} rows={[...rows]} getRowKey={getRowKey} emptyMessage={emptyMessage} />;
  }

  if (mode === 'cards') {
    return (
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {rows.map((row) => {
          const key = getRowKey(row);
          return (
            <Card key={key} className={onRowClick ? 'cursor-pointer transition-colors hover:border-primary/50' : undefined} onClick={() => onRowClick?.(row)}>
              <CardContent className="flex items-start gap-3 p-4">
                {selectable && <Checkbox aria-label={`Select row ${key}`} checked={selection.includes(key)} onClick={(event) => event.stopPropagation()} onCheckedChange={(checked) => toggleRow(key, checked === true)} />}
                <div className="min-w-0 flex-1">{renderItem ? renderItem(row) : columns.map((column) => <div key={column.id} className="flex justify-between gap-4 py-1 text-sm"><span className="text-muted-foreground">{column.header}</span><span>{column.cell(row)}</span></div>)}</div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    );
  }

  return (
    <div className="divide-y rounded-lg border">
      {rows.map((row) => {
        const key = getRowKey(row);
        return (
          <div key={key} className="flex items-start gap-3 p-4">
            {selectable && <Checkbox aria-label={`Select row ${key}`} checked={selection.includes(key)} onCheckedChange={(checked) => toggleRow(key, checked === true)} />}
            {onRowClick ? (
              <Button variant="ghost" className="h-auto min-w-0 flex-1 justify-start p-0 text-left" onClick={() => onRowClick(row)}>
                {renderItem ? renderItem(row) : <div className="grid min-w-0 gap-2 sm:grid-cols-2">{columns.map((column) => <div key={column.id} className="min-w-0"><div className="text-xs text-muted-foreground">{column.header}</div><div className="truncate text-sm">{column.cell(row)}</div></div>)}</div>}
              </Button>
            ) : (
              <div className="min-w-0 flex-1">
                {renderItem ? renderItem(row) : <div className="grid min-w-0 gap-2 sm:grid-cols-2">{columns.map((column) => <div key={column.id} className="min-w-0"><div className="text-xs text-muted-foreground">{column.header}</div><div className="truncate text-sm">{column.cell(row)}</div></div>)}</div>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
