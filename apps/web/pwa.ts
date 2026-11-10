import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Plugin } from 'vite';

/** Cache only the build's public shell. Account data lives in IndexedDB. */
export function offlineShell(): Plugin {
  let root = '';
  return {
    name: 'taskboard-offline-shell',
    apply: 'build',
    configResolved(config) {
      root = config.root;
    },
    generateBundle(_options, bundle) {
      const files = Object.keys(bundle).filter((name) => !name.endsWith('.map'));
      const assets = [
        ...new Set([
          '/index.html',
          ...files.map((name) => `/${name}`),
          '/manifest.webmanifest',
          '/icon.svg',
          '/icon-192.png',
          '/icon-512.png',
        ]),
      ];
      const version = createHash('sha256');
      for (const name of files.sort()) {
        const asset = bundle[name];
        version.update(name).update(asset.type === 'chunk' ? asset.code : asset.source);
      }
      if (root) {
        version.update(readFileSync(resolve(root, 'index.html')));
        for (const name of ['manifest.webmanifest', 'icon.svg', 'icon-192.png', 'icon-512.png']) {
          version.update(readFileSync(resolve(root, 'public', name)));
        }
      }
      const hash = version.digest('hex').slice(0, 16);
      this.emitFile({
        type: 'asset',
        fileName: 'sw.js',
        source: `const CACHE = 'taskboard-shell-${hash}';
const ASSETS = ${JSON.stringify(assets)};
self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(ASSETS)));
});
// Wait for old tabs to close before activating a new shell.
self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(
    keys.filter((key) => key.startsWith('taskboard-shell-') && key !== CACHE)
      .map((key) => caches.delete(key))
  )).then(() => self.clients.claim()));
});
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  const networkPaths = ['/api', '/caldav', '/carddav', '/_', '/docs', '/openapi.json', '/openapi.yaml'];
  if (event.request.method !== 'GET' || url.origin !== self.location.origin || networkPaths.some((prefix) => url.pathname === prefix || url.pathname.startsWith(prefix + '/'))) return;
  if (event.request.mode === 'navigate') {
    event.respondWith(caches.open(CACHE).then((cache) => cache.match('/index.html')).then((cached) => cached || fetch(event.request)));
  } else if (ASSETS.includes(url.pathname)) {
    event.respondWith(caches.open(CACHE).then((cache) => cache.match(url.pathname)).then((cached) => cached || fetch(event.request)));
  }
});`,
      });
    },
  };
}
