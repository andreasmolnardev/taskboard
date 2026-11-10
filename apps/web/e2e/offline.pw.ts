import { expect, test } from '@playwright/test';

test('installed shell and queued task survive an offline reload and sync on reconnect', async ({
  page,
  context,
}) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  const owner = 'offlineuser0001';
  const list = { id: 'offlinelist0001', owner, name: 'Tasks', color: '#123456', description: '' };
  const changes: Array<Record<string, unknown>> = [
    { collection: 'lists', id: list.id, revision: 1, deleted: false, record: list },
  ];
  const receipts = new Map<string, Record<string, unknown>>();
  await context.addInitScript(
    ({ owner }) => {
      if (!localStorage.getItem('pocketbase_auth')) {
        const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 86400 }));
        localStorage.setItem(
          'pocketbase_auth',
          JSON.stringify({
            token: `header.${payload}.signature`,
            record: { id: owner, email: 'offline@example.com', collectionName: 'users' },
          }),
        );
      }
    },
    { owner },
  );
  await context.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/sync/pull') {
      const cursor = Number(url.searchParams.get('cursor') ?? 0);
      await route.fulfill({
        json: {
          cursor: changes.length,
          changes: changes.filter((change) => Number(change.revision) > cursor),
        },
      });
    } else if (url.pathname === '/api/sync/push') {
      const results = [];
      for (const op of route.request().postDataJSON().operations) {
        let result = receipts.get(op.operationId);
        if (!result) {
          const record =
            op.kind === 'delete' ? null : { ...op.base, ...op.patch, id: op.id, owner };
          const revision = changes.length + 1;
          changes.push({
            collection: op.collection,
            id: op.id,
            revision,
            record,
            deleted: op.kind === 'delete',
          });
          result = {
            operationId: op.operationId,
            status: 'applied',
            revision,
            record,
            deleted: op.kind === 'delete',
          };
          receipts.set(op.operationId, result);
        }
        results.push(result);
      }
      await route.fulfill({ json: { results, cursor: changes.length } });
    } else if (url.pathname === '/api/realtime') {
      await route.abort();
    } else {
      await route.fulfill({ json: [] });
    }
  });
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'New entry', exact: true })).toBeVisible();
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
  });
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);
  await expect.poll(() => changes.length).toBe(1);
  // Wait until the local snapshot has been loaded before cutting the connection.
  await expect(page.getByText(/Last synced/)).toBeVisible();
  const peer = await context.newPage();
  peer.on('pageerror', (error) => pageErrors.push(error.message));
  await peer.goto('/lists');
  await context.setOffline(true);
  await expect(page.getByRole('status').filter({ hasText: 'Offline' })).toBeVisible();
  await page.getByRole('button', { name: 'New entry', exact: true }).click();
  await page.getByPlaceholder('What needs doing?').fill('Saved while offline');
  await page.getByRole('button', { name: 'Create task', exact: true }).click();
  await expect(page.getByText('Saved while offline', { exact: true }).first()).toBeVisible();
  await expect(peer.getByText('Saved while offline', { exact: true }).first()).toBeVisible();
  await page.reload();
  await expect(page.getByText('Saved while offline', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Edit Saved while offline', exact: true }).click();
  await page.getByPlaceholder('What needs doing?').fill('Edited while offline');
  await page.getByRole('button', { name: 'Save changes', exact: true }).click();
  await expect(page.getByText('Edited while offline', { exact: true }).first()).toBeVisible();
  await page.reload();
  await expect(page.getByText('Edited while offline', { exact: true }).first()).toBeVisible();
  expect(changes).toHaveLength(1);
  await context.setOffline(false);
  await expect.poll(() => changes.filter((change) => change.collection === 'todos').length).toBe(1);
  await expect(page.getByText(/1 pending/)).toHaveCount(0);
  expect((changes[1].record as Record<string, unknown>).title).toBe('Edited while offline');
  await page.goto('/lists');
  await context.setOffline(true);
  await page.getByRole('button', { name: 'Complete Edited while offline', exact: true }).click();
  await expect(
    page.getByRole('button', { name: 'Reopen Edited while offline', exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole('button', { name: 'Reopen Edited while offline', exact: true }),
  ).toBeVisible();
  await context.setOffline(false);
  await expect.poll(() => changes.length).toBe(3);
  expect((changes[2].record as Record<string, unknown>).completed).toBe(true);
  await context.setOffline(true);
  page.once('dialog', (dialog) => void dialog.accept());
  await page.getByRole('button', { name: 'Delete Edited while offline', exact: true }).click();
  await expect(page.getByText('Edited while offline', { exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByText('Edited while offline', { exact: true })).toHaveCount(0);
  await context.setOffline(false);
  await expect.poll(() => changes.length).toBe(4);
  expect(changes[3].deleted).toBe(true);
  expect(pageErrors).toEqual([]);
});
