import { useEffect, useId, useState, type FormEvent } from 'react';
import {
  Archive,
  ArchiveRestore,
  CalendarDays,
  Eye,
  EyeOff,
  ListTodo,
  Pencil,
  Trash2,
  X,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';

export type ManagedContainer = {
  id: string;
  name: string;
  color: string;
  description: string;
  kind: 'list' | 'calendar';
  count: number;
  visible: boolean;
  archived: boolean;
};

export type ContainerChanges = Pick<ManagedContainer, 'name' | 'color' | 'description'>;
export type RemovalAction = 'delete' | 'archive';

export type ContainerManagerProps = {
  containers: readonly ManagedContainer[];
  defaultSelectedId?: string;
  onSelect?: (container: ManagedContainer) => void;
  onUpdate: (container: ManagedContainer, changes: ContainerChanges) => void | Promise<void>;
  onVisibilityChange: (container: ManagedContainer, visible: boolean) => void | Promise<void>;
  onArchiveChange: (container: ManagedContainer, archived: boolean) => void | Promise<void>;
  onRemove: (container: ManagedContainer, action: RemovalAction) => void | Promise<void>;
};

export function getSafeRemovalAction(container: Pick<ManagedContainer, 'count'>): RemovalAction {
  return container.count > 0 ? 'archive' : 'delete';
}

export function resolveDefaultSelection(
  containers: readonly Pick<ManagedContainer, 'id'>[],
  requestedId?: string,
): string | undefined {
  return containers.some(({ id }) => id === requestedId) ? requestedId : containers[0]?.id;
}

export function ContainerManager({
  containers,
  defaultSelectedId,
  onSelect,
  onUpdate,
  onVisibilityChange,
  onArchiveChange,
  onRemove,
}: ContainerManagerProps) {
  const initialId = resolveDefaultSelection(containers, defaultSelectedId);
  const [selectedId, setSelectedId] = useState(initialId);
  const [editingId, setEditingId] = useState<string>();
  const [removeId, setRemoveId] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [archivingId, setArchivingId] = useState<string>();
  const [error, setError] = useState('');
  const nameId = useId();
  const colorId = useId();
  const descriptionId = useId();

  useEffect(() => {
    if (!containers.some(({ id }) => id === selectedId)) {
      setSelectedId(resolveDefaultSelection(containers, defaultSelectedId));
    }
  }, [containers, defaultSelectedId, selectedId]);

  const selected = containers.find(({ id }) => id === selectedId);
  const editing = containers.find(({ id }) => id === editingId);
  const pendingRemoval = containers.find(({ id }) => id === removeId);
  const removalAction = pendingRemoval ? getSafeRemovalAction(pendingRemoval) : undefined;

  function select(container: ManagedContainer) {
    setSelectedId(container.id);
    onSelect?.(container);
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing) return;

    const data = new FormData(event.currentTarget);
    const name = String(data.get('name') ?? '').trim();
    if (!name) return;

    setSaving(true);
    setError('');
    try {
      await onUpdate(editing, {
        name,
        color: String(data.get('color') ?? editing.color),
        description: String(data.get('description') ?? '').trim(),
      });
      setEditingId(undefined);
    } catch {
      setError(`Could not update ${editing.kind} ${editing.name}.`);
    } finally {
      setSaving(false);
    }
  }

  async function changeVisibility(container: ManagedContainer, visible: boolean) {
    setError('');
    try {
      await onVisibilityChange(container, visible);
    } catch {
      setError(`Could not change the visibility of ${container.name}.`);
    }
  }

  async function changeArchived(container: ManagedContainer, archived: boolean) {
    setArchivingId(container.id);
    setError('');
    try {
      await onArchiveChange(container, archived);
    } catch {
      setError(`Could not ${archived ? 'archive' : 'unarchive'} ${container.name}.`);
    } finally {
      setArchivingId(undefined);
    }
  }

  async function remove(container: ManagedContainer, action: RemovalAction) {
    setRemoving(true);
    setError('');
    try {
      await onRemove(container, action);
      setRemoveId(undefined);
    } catch {
      setError(`Could not ${action === 'archive' ? 'archive' : 'delete'} ${container.name}.`);
    } finally {
      setRemoving(false);
    }
  }

  return (
    <section className="grid gap-4" aria-labelledby="container-manager-title">
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div>
        <h2 id="container-manager-title" className="text-lg font-semibold">
          Lists and calendars
        </h2>
        <p className="text-sm text-muted-foreground">Choose what is shown and edit its details.</p>
      </div>

      {containers.length === 0 ? (
        <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          No lists or calendars yet.
        </p>
      ) : (
        <div className="grid gap-2" role="list" aria-label="Lists and calendars">
          {containers.map((container) => {
            const Icon = container.kind === 'list' ? ListTodo : CalendarDays;
            const isSelected = container.id === selected?.id;
            return (
              <article
                key={container.id}
                role="listitem"
                className={`grid grid-cols-[auto_1fr_auto] items-center gap-3 rounded-md border p-3 ${isSelected ? 'border-primary bg-accent/40' : ''}`}
              >
                <button
                  type="button"
                  className="flex min-w-0 items-center gap-3 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-pressed={isSelected}
                  aria-label={`Select ${container.kind} ${container.name}`}
                  onClick={() => select(container)}
                >
                  <span
                    className="flex size-9 shrink-0 items-center justify-center rounded-full"
                    style={{ backgroundColor: container.color }}
                    aria-hidden="true"
                  >
                    <Icon className="size-4 text-white" />
                  </span>
                  <span className="min-w-0">
                    <strong className="block truncate text-sm">{container.name}</strong>
                    <span className="block truncate text-xs text-muted-foreground">
                      {container.description ||
                        `No description · ${container.count} ${container.count === 1 ? 'item' : 'items'}`}
                    </span>
                    {container.archived && (
                      <span className="block text-xs font-medium text-muted-foreground">
                        Archived
                      </span>
                    )}
                  </span>
                </button>

                <div className="flex items-center gap-2">
                  {container.visible ? (
                    <Eye className="size-4" aria-hidden="true" />
                  ) : (
                    <EyeOff className="size-4" aria-hidden="true" />
                  )}
                  <Label htmlFor={`visibility-${container.id}`} className="sr-only">
                    Show {container.name}
                  </Label>
                  <Switch
                    id={`visibility-${container.id}`}
                    checked={container.visible}
                    aria-label={`Show ${container.name}`}
                    disabled={container.archived}
                    onCheckedChange={(visible) => void changeVisibility(container, visible)}
                  />
                </div>

                <div className="flex gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Edit ${container.name}`}
                    onClick={() => setEditingId(container.id)}
                  >
                    <Pencil />
                  </Button>
                  {container.archived ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Unarchive ${container.name}`}
                      onClick={() => void changeArchived(container, false)}
                      disabled={archivingId === container.id}
                    >
                      <ArchiveRestore />
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Remove ${container.name}`}
                      onClick={() => setRemoveId(container.id)}
                    >
                      <Trash2 />
                    </Button>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}

      {editing && (
        <div
          className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4"
          role="presentation"
        >
          <section
            className="w-full max-w-md rounded-lg border bg-background p-5 shadow-lg"
            role="dialog"
            aria-modal="true"
            aria-labelledby="edit-container-title"
          >
            <div className="mb-4 flex items-center justify-between">
              <h3 id="edit-container-title" className="font-semibold">
                Edit {editing.kind}
              </h3>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Close edit form"
                onClick={() => setEditingId(undefined)}
              >
                <X />
              </Button>
            </div>
            <form className="grid gap-4" onSubmit={(event) => void save(event)}>
              <div className="grid gap-2">
                <Label htmlFor={nameId}>Name</Label>
                <Input id={nameId} name="name" required defaultValue={editing.name} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor={colorId}>Color</Label>
                <Input
                  id={colorId}
                  name="color"
                  type="color"
                  defaultValue={editing.color}
                  className="w-20 p-1"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor={descriptionId}>Description</Label>
                <Textarea
                  id={descriptionId}
                  name="description"
                  defaultValue={editing.description}
                />
              </div>
              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => setEditingId(undefined)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={saving}>
                  {saving ? 'Saving…' : 'Save changes'}
                </Button>
              </div>
            </form>
          </section>
        </div>
      )}

      {pendingRemoval && removalAction && (
        <div
          className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4"
          role="presentation"
        >
          <section
            className="w-full max-w-md rounded-lg border bg-background p-5 shadow-lg"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="remove-container-title"
            aria-describedby="remove-container-description"
          >
            <h3 id="remove-container-title" className="font-semibold">
              {removalAction === 'archive'
                ? `Archive ${pendingRemoval.name}?`
                : `Delete ${pendingRemoval.name}?`}
            </h3>
            <p id="remove-container-description" className="mt-2 text-sm text-muted-foreground">
              {removalAction === 'archive'
                ? `This ${pendingRemoval.kind} has ${pendingRemoval.count} ${pendingRemoval.count === 1 ? 'item' : 'items'}, so it will be archived instead of deleted.`
                : `This empty ${pendingRemoval.kind} will be deleted. This cannot be undone.`}
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => setRemoveId(undefined)}>
                Cancel
              </Button>
              <Button
                type="button"
                variant={removalAction === 'delete' ? 'destructive' : 'default'}
                onClick={() => void remove(pendingRemoval, removalAction)}
                disabled={removing}
              >
                {removalAction === 'archive' ? <Archive /> : <Trash2 />}
                {removing ? 'Working…' : removalAction === 'archive' ? 'Archive' : 'Delete'}
              </Button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}
