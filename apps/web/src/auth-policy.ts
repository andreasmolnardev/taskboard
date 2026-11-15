export type OAuthMethods = {
  oauth2?: { enabled?: boolean; providers?: Array<{ name: string; displayName?: string }> };
};

export function enabledSsoProviders(methods: OAuthMethods) {
  if (!methods.oauth2?.enabled) return [];
  return (methods.oauth2.providers ?? []).map(({ name, displayName }) => ({ name, displayName }));
}
