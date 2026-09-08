# Navigation

## Desktop

Desktop uses a persistent left sidebar and a sticky top bar.

| Area        | Route             | Purpose                              |
| ----------- | ----------------- | ------------------------------------ |
| Overview    | `/app/overview`   | Example module summary and task list |
| Tasks       | `/app/tasks`      | Domain task management screen        |
| Schedule    | `/app/schedule`   | Calendar and time-oriented work      |
| Analytics   | `/app/analytics`  | Reporting and progress metrics       |
| Components  | `/app/components` | UI primitive catalog                 |
| Settings    | `/app/profile`    | Account and workspace settings       |
| Admin users | `/admin/users`    | Superuser user management            |
| Admin SSO   | `/admin/sso`      | PocketBase OAuth2 provider setup     |
| API docs    | `/docs`           | OpenAPI documentation served by Go   |

The frontend uses browser history routes so each workspace view has a shareable deep link. Unauthenticated app paths redirect to `/auth/login` with a return path.

## Mobile

The sidebar collapses below the `lg` breakpoint. A fixed bottom navigation exposes workspace views. Account actions remain in the top-right profile control.

## Interaction rules

- Active navigation uses shadcn `Button` with `secondary` variant.
- Primary creation uses one visible `Add task` action.
- Destructive actions stay secondary and require clear labels.
- API docs open in a separate tab from the sidebar.
