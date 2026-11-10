import { createElement, Fragment, type ReactNode } from 'react';
import type { LayoutDefinition, LayoutSlot, ShellDefinition } from './types';

export type LayoutItem = {
  kind: NonNullable<LayoutSlot['accepts']>[number];
  content: ReactNode;
};

export type LayoutContents = Readonly<Record<string, LayoutItem | readonly LayoutItem[] | undefined>>;

/** Validates slot requirements and renders a layout from declarative, typed contents. */
export function renderLayout(layout: LayoutDefinition, contents: LayoutContents = {}) {
  const declared = new Map(layout.slots.map((slot) => [slot.id, slot]));
  for (const key of Object.keys(contents)) {
    if (!declared.has(key)) throw new Error(`Layout "${layout.id}" has no slot "${key}".`);
  }

  const slots: Record<string, ReactNode> = {};
  for (const slot of layout.slots) {
    const input = contents[slot.id];
    const items = input === undefined ? [] : Array.isArray(input) ? input : [input];
    if (slot.required && items.length === 0) throw new Error(`Layout "${layout.id}" requires slot "${slot.id}".`);
    if (!slot.multiple && items.length > 1) throw new Error(`Layout "${layout.id}" slot "${slot.id}" accepts one item.`);
    for (const item of items) {
      if (slot.accepts?.length && !slot.accepts.includes(item.kind)) {
        throw new Error(`Layout "${layout.id}" slot "${slot.id}" does not accept ${item.kind}.`);
      }
    }
    slots[slot.id] = items.map((item) => item.content);
  }
  return createElement(layout.component, { slots });
}

/** Keeps shell-owned regions mounted alongside the changing route content. */
export function renderShell(shell: ShellDefinition, children: ReactNode) {
  const regions = (shell.persistentRegions ?? []).map(({ id, component: Region }) => createElement(Region, { key: id }));
  return createElement(shell.component, { children: createElement(Fragment, null, children, ...regions) });
}
