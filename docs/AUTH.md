# Authentication

PocketBase owns the auth lifecycle.

## Built-in flows

- Login and logout.
- Registration, controlled by `SLOPSTACK_REGISTRATION_ENABLED`.
- Password reset.
- Email verification.
- Persistent auth tokens.
- Auth record access through `pb.authStore`.

## Frontend pattern

```ts
await pb.collection('users').authWithPassword(email, password);
const user = pb.authStore.record;
pb.authStore.clear();
```

Reference routes are `/auth/login`, `/auth/register`, `/auth/forgot-password`, and `/auth/admin`. App routes require a valid PocketBase auth record and redirect back after login. Admin routes require a PocketBase superuser session. Logout clears `pb.authStore` and returns to `/auth/login`.

The admin console at `/admin/users` manages users through PocketBase's superuser-protected collection API. `/admin/sso` configures OAuth2 providers on the `users` auth collection. Register each provider with `{origin}/api/oauth2-redirect`.

## Backend pattern

Protected routes must reject missing `event.Auth` and apply `Authorize` for permission checks. Frontend navigation and conditional rendering are not security controls.
