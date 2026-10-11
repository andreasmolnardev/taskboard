import { pb } from './pocketbase';

const configuredApiUrl = import.meta.env.VITE_API_URL as string | undefined;

export const apiBaseUrl = (configuredApiUrl || window.location.origin).replace(/\/$/, '');

export type ApiRequestInit = RequestInit & { auth?: boolean };

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly payload?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function apiFetch(path: string, init: ApiRequestInit = {}): Promise<Response> {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  const { auth = true, headers: suppliedHeaders, ...requestInit } = init;
  const headers = new Headers(suppliedHeaders);
  if (auth && pb.authStore.token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${pb.authStore.token}`);
  }
  return fetch(`${apiBaseUrl}${normalizedPath}`, { ...requestInit, headers });
}

export async function apiJson<T>(path: string, init?: ApiRequestInit): Promise<T> {
  const response = await apiFetch(path, init);
  const payload = await response.json().catch(() => undefined);
  if (!response.ok) {
    const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
    const message =
      (typeof record.message === 'string' && record.message) ||
      (typeof record.error === 'string' && record.error) ||
      response.statusText ||
      'The request failed.';
    throw new ApiError(message, response.status, payload);
  }
  return payload as T;
}
