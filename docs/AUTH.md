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

## Account data controls

Signed-in users can download their account data from Settings. The export omits passwords, auth tokens, and hidden app-password hashes. Users can also permanently delete their account after typing their account email to confirm. The API scopes both actions to the authenticated user; account deletion removes owned records in a database transaction.

See [API account data](API.md#account-data) for endpoint details and [Deployment upgrades and rollback](DEPLOYMENT.md#upgrades-and-rollback) for release and migration recovery steps.

## MCP OAuth

The task server can expose an OAuth-protected MCP endpoint for per-user access. Set `SLOPSTACK_PUBLIC_URL` to the public HTTPS origin and set `SLOPSTACK_MCP_OAUTH_SIGNING_KEY` to a random secret with at least 32 characters. Keep this key private and stable; changing it invalidates active MCP access tokens. The endpoint is `/mcp` by default and can be changed with `SLOPSTACK_MCP_PATH`. Set `SLOPSTACK_MCP_RESOURCE_URL` only when the server's canonical MCP resource URL differs from the default.

With a valid signing key, MCP clients discover OAuth metadata and can register as public clients. Users sign in to Taskboard and approve access. Access tokens last one hour and are bound to the MCP resource. The first tools support listing, reading, creating, and updating the signed-in user's tasks. Refresh tokens and revocation are not supported yet.
