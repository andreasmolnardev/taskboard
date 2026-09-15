const configuredApiUrl = import.meta.env.VITE_API_URL as string | undefined;

export const apiBaseUrl = (configuredApiUrl || window.location.origin).replace(/\/$/, '');

export function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return fetch(`${apiBaseUrl}${normalizedPath}`, init);
}
