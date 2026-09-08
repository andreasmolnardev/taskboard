# Deployment

## Docker

```sh
docker compose up --build
```

PocketBase data persists in the `slopstack-data` volume.

## Required production settings

- Set `SLOPSTACK_PUBLIC_URL` to the public origin.
- Set `SLOPSTACK_SECURE_COOKIES=true` behind HTTPS.
- Set a durable `SLOPSTACK_STORAGE_PATH`.
- Keep PocketBase data outside ephemeral container storage.

## Frontend

The frontend is a static Vite build. Keep `/api` proxied to the Go runtime or set `VITE_API_URL` to the deployed API origin.
