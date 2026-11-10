import { describe, expect, it } from 'vitest';
import vm from 'node:vm';
import { offlineShell } from './pwa';

function serviceWorker() {
  let source = '';
  const hook = offlineShell().generateBundle as Function;
  hook.call(
    {
      emitFile: (asset: { source: string }) => {
        source = asset.source;
      },
    },
    {},
    {
      'index.html': { type: 'asset', source: '<html></html>' },
      'assets/app-a.js': { type: 'chunk', code: 'app()' },
      'assets/app-a.js.map': { type: 'asset', source: '{}' },
    },
  );
  const handlers: Record<string, Function> = {};
  const stored = new Map<string, string>();
  const cache = {
    addAll: async (assets: string[]) =>
      assets.forEach((asset) => stored.set(asset, `cached:${asset}`)),
    match: async (path: string) => stored.get(path),
  };
  vm.runInNewContext(source, {
    URL,
    self: {
      location: { origin: 'https://taskboard.test' },
      clients: { claim: async () => undefined },
      addEventListener: (name: string, fn: Function) => {
        handlers[name] = fn;
      },
    },
    caches: { open: async () => cache },
    fetch: async () => {
      throw new Error('offline');
    },
  });
  return { handlers, stored };
}

describe('offline app shell', () => {
  it('precaches build assets and serves navigation offline', async () => {
    const { handlers, stored } = serviceWorker();
    let install: Promise<unknown> | undefined;
    handlers.install({
      waitUntil: (promise: Promise<unknown>) => {
        install = promise;
      },
    });
    await install;
    expect(stored.has('/assets/app-a.js')).toBe(true);
    expect(stored.has('/assets/app-a.js.map')).toBe(false);
    let response: Promise<unknown> | undefined;
    handlers.fetch({
      request: { url: 'https://taskboard.test/calendar', method: 'GET', mode: 'navigate' },
      respondWith: (promise: Promise<unknown>) => {
        response = promise;
      },
    });
    await expect(response).resolves.toBe('cached:/index.html');
  });

  it('leaves authenticated API requests and other origins to the network', () => {
    const { handlers } = serviceWorker();
    for (const url of ['https://taskboard.test/api/sync/pull', 'https://other.test/index.html']) {
      handlers.fetch({
        request: { url, method: 'GET', mode: 'navigate' },
        respondWith: () => {
          throw new Error('Private requests must not use shell cache');
        },
      });
    }
  });
});
