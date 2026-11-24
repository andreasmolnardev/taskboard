# Authentication

PocketBase owns the auth lifecycle.

## Built-in flows

- Login and logout.
- Registration, controlled by `TASKBOARD_REGISTRATION_ENABLED` (with `SLOPSTACK_REGISTRATION_ENABLED` accepted as a legacy fallback).
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

The admin console at `/admin/users` manages users through PocketBase's superuser-protected collection API. `/admin/sso` configures OAuth2 providers on the `users` auth collection. Its Taskboard API returns provider settings without client secrets and preserves saved secrets when a blank secret is submitted. Other providers remain configured when editing OIDC. Custom OIDC uses discovery or explicit HTTPS endpoints; issuer and JWKS are required, and claims come from signed ID tokens so PocketBase validates issuer and signature. PocketBase's OAuth client flow supplies state and PKCE; its native OIDC implementation does not add a nonce. The callback follows the frontend PocketBase URL (`VITE_API_URL`, or the page origin when unset). Register each provider with `{origin}/api/oauth2-redirect`.

`SLOPSTACK_REGISTRATION_ENABLED=false` blocks public password registration and new OAuth accounts while existing users can still sign in. Taskboard keeps its approval and OTP registration modes. `SLOPSTACK_SSO_ONLY=true` blocks password sign-in only when OAuth is enabled with at least one provider, to avoid locking out all users. `SLOPSTACK_REQUIRE_VERIFIED_EMAIL=true` requires verified email at authentication; password signups receive a PocketBase verification email.

User email addresses are normalized to lowercase and checked case-insensitively to prevent duplicate accounts. Authenticated requests reload the account so disabling a user revokes older tokens and closes that user's active realtime subscriptions. Users cannot change their own disabled state. An unauthenticated OAuth sign-in is accepted only for an identity already linked to that account. Matching email alone never links an identity; users must sign in first and confirm linking with their password and a five-minute, one-use intent. New OAuth accounts require a provider email when verified email is required. The account screen offers password recovery for OAuth-only users, and both the UI and server prevent unlinking the last connected SSO identity. Custom OIDC providers use PocketBase's `oidc2` provider configuration with issuer and JWKS values for token validation.

## Backend pattern

Protected routes must reject missing `event.Auth` and apply `Authorize` for permission checks. Frontend navigation and conditional rendering are not security controls.

## Account data controls

Signed-in users can download their account data from Settings. The export omits passwords, auth tokens, and hidden app-password hashes. Users can also permanently delete their account after typing their account email to confirm. The API scopes both actions to the authenticated user; account deletion removes owned records in a database transaction.

See [API account data](API.md#account-data) for endpoint details and [Deployment upgrades and rollback](DEPLOYMENT.md#upgrades-and-rollback) for release and migration recovery steps.

## MCP OAuth

The task server can expose an OAuth-protected MCP endpoint for per-user access. Set `SLOPSTACK_PUBLIC_URL` to the public HTTPS origin and set `SLOPSTACK_MCP_OAUTH_SIGNING_KEY` to a random secret with at least 32 characters. Keep this key private and stable; changing it invalidates active MCP access tokens. The endpoint is `/mcp` by default and can be changed with `SLOPSTACK_MCP_PATH`. Set `SLOPSTACK_MCP_RESOURCE_URL` only when the server's canonical MCP resource URL differs from the default.

With a valid signing key, MCP clients discover OAuth metadata and can register as public clients. Users sign in to Taskboard and approve access using PKCE. Access tokens last one hour and are bound to the MCP resource. Refresh tokens are rotated on every use and expire after 30 days. Users can inspect and revoke authorized clients in Settings; revocation invalidates that user's sessions immediately.

Task and project MCP tools are documented in [MCP.md](MCP.md). Calendar and contact tool coverage remains in progress.
