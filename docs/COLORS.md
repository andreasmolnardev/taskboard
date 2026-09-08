# Colors

## Intent

Use a quiet light workspace surface with high-contrast ink, neutral cards, and restrained semantic accents. The task UI is a visual reference, not a fixed product brand.

## Tokens

Tokens live in `apps/web/src/styles.css` and map to shadcn semantic names:

| Token | Use |
| --- | --- |
| `background` | App canvas and page background |
| `foreground` | Primary text and icons |
| `card` | Panels, cards, and elevated surfaces |
| `muted` | Secondary surfaces and inactive controls |
| `muted-foreground` | Supporting copy and metadata |
| `primary` | Primary action and selected navigation |
| `border` | Dividers and field outlines |
| `destructive` | Destructive actions and error states |

## Semantic accents

- Blue: active work and open-task metrics.
- Green: completion and healthy status.
- Violet: people, teams, and collaboration.
- Rose: high priority and paid-plan emphasis.
- Amber: medium priority and attention states.
- Sky: low priority and informational states.

Prefer semantic tokens over raw colors. Add a new token only when an existing semantic role cannot express the state.
