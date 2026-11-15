import { describe, expect, it } from 'vitest';
import { enabledSsoProviders } from './auth-policy';

describe('enabledSsoProviders', () => {
  it('hides providers while PocketBase OAuth is disabled', () => {
    expect(enabledSsoProviders({ oauth2: { enabled: false, providers: [{ name: 'google' }] } })).toEqual([]);
  });

  it('returns enabled providers and display names', () => {
    expect(enabledSsoProviders({ oauth2: { enabled: true, providers: [{ name: 'oidc2', displayName: 'Company SSO' }] } })).toEqual([
      { name: 'oidc2', displayName: 'Company SSO' },
    ]);
  });
});
