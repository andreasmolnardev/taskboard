# Components

## Source

All reusable UI primitives live in `apps/web/src/components/ui`. They are shadcn components backed by Radix primitives where interaction behavior needs accessibility and keyboard support.

## Common composition

- `Card`: page panels, metrics, profiles, and grouped settings.
- `Button`: navigation, primary actions, icon actions, and secondary actions.
- `Dialog`: create and edit workflows.
- `Input`, `Textarea`, `Select`, `Label`: forms.
- `Checkbox`: task completion.
- `Tabs`: task filters.
- `Badge`: module labels, priority, and status.
- `Avatar`: people and assignee stacks.
- `Progress`: delivery metrics.
- `Separator`: visual grouping.

## Rules

- Import primitives through `@/components/ui/*`.
- Use `cn` from `@/lib/utils` for conditional classes.
- Keep product-specific composition in feature files, not inside primitives.
- Add new primitives with shadcn CLI when available.
- Wrap the app with `TooltipProvider` before using tooltip primitives.
- Generated components may be adjusted for project lint rules, but preserve their public API.

## Adding a component

```sh
pnpm dlx shadcn@latest add dialog -c apps/web
```

The current registry has some unavailable legacy names. Use `sonner` instead of Base UI-only `toast` in this Radix setup.
