import assert from 'node:assert/strict';
import test from 'node:test';
import type * as Updates from 'expo-updates';
import { createOtaController } from './ota-controller';

type Port = Pick<typeof Updates, 'checkForUpdateAsync' | 'fetchUpdateAsync' | 'reloadAsync'>;
const noUpdate = { isAvailable: false, isRollBackToEmbedded: false, reason: 'noUpdateAvailableOnServer' } as Updates.UpdateCheckResult;
const available = { isAvailable: true, isRollBackToEmbedded: false, manifest: {} } as Updates.UpdateCheckResult;
const rollback = { isAvailable: false, isRollBackToEmbedded: true } as Updates.UpdateCheckResult;
const downloaded = { isNew: true, isRollBackToEmbedded: false, manifest: {} } as Updates.UpdateFetchResult;

function setup(overrides: Partial<Port> = {}) {
  let now = 1;
  const calls = { check: 0, fetch: 0, reload: 0 };
  const port: Port = {
    async checkForUpdateAsync() { calls.check += 1; return noUpdate; },
    async fetchUpdateAsync() { calls.fetch += 1; return downloaded; },
    async reloadAsync() { calls.reload += 1; },
    ...overrides,
  };
  const controller = createOtaController(port, { now: () => now, random: () => 0 });
  return { controller, calls, advance: (ms: number) => { now += ms; } };
}

test('deduplicates concurrent foreground/manual checks until the native promise settles', async () => {
  let release!: (value: Updates.UpdateCheckResult) => void;
  let calls = 0;
  const { controller } = setup({
    checkForUpdateAsync() { calls += 1; return new Promise(resolve => { release = resolve; }); },
  });
  const first = controller.checkAndDownload('automatic');
  await controller.checkAndDownload('manual');
  assert.equal(calls, 1);
  assert.equal(controller.getSnapshot().phase, 'checking');
  release(noUpdate);
  await first;
  assert.equal(controller.getSnapshot().phase, 'idle');
});

test('rollback is fetched and requires an explicit apply even when isAvailable is false', async () => {
  const { controller, calls } = setup({
    async checkForUpdateAsync() { return rollback; },
    async fetchUpdateAsync() { calls.fetch += 1; return { isNew: false, isRollBackToEmbedded: true, manifest: undefined }; },
  });
  await controller.checkAndDownload();
  assert.equal(controller.getSnapshot().phase, 'ready');
  assert.equal(calls.fetch, 1);
  assert.equal(calls.reload, 0);
  await controller.apply();
  assert.equal(calls.reload, 1);
});

test('failed download stays recoverable, throttles automatic retry, and permits a later user retry', async () => {
  let attempts = 0;
  const { controller, advance } = setup({
    async checkForUpdateAsync() { attempts += 1; return available; },
    async fetchUpdateAsync() { throw new Error('offline or invalid signature'); },
  });
  await controller.checkAndDownload('automatic');
  assert.equal(controller.getSnapshot().phase, 'error');
  await controller.apply();
  advance(31_000);
  await controller.checkAndDownload('automatic');
  assert.equal(attempts, 1);
  await controller.checkAndDownload('manual');
  assert.equal(attempts, 2);
});

test('healthy foreground checks are limited to once per fifteen minutes', async () => {
  const { controller, calls, advance } = setup();
  await controller.checkAndDownload('automatic');
  advance(14 * 60_000);
  await controller.checkAndDownload('automatic');
  assert.equal(calls.check, 1);
  advance(60_000);
  await controller.checkAndDownload('automatic');
  assert.equal(calls.check, 2);
});

test('ready update survives reload failure and can be explicitly retried', async () => {
  let restarts = 0;
  const { controller } = setup({
    async checkForUpdateAsync() { return available; },
    async reloadAsync() { restarts += 1; if (restarts === 1) throw new Error('reload unavailable'); },
  });
  await controller.checkAndDownload();
  await controller.apply();
  assert.equal(controller.getSnapshot().phase, 'ready');
  assert.match(controller.getSnapshot().error!.message, /reload unavailable/);
  await controller.apply();
  assert.equal(restarts, 2);
});

test('does not replace a pending download and handles a release withdrawn between check and fetch', async () => {
  let checks = 0;
  const { controller, advance } = setup({
    async checkForUpdateAsync() { checks += 1; return available; },
  });
  await controller.checkAndDownload();
  advance(60 * 60_000);
  await controller.checkAndDownload();
  assert.equal(checks, 1);

  const withdrawn = setup({
    async checkForUpdateAsync() { return available; },
    async fetchUpdateAsync() { return { isNew: false, isRollBackToEmbedded: false, manifest: undefined }; },
  });
  await withdrawn.controller.checkAndDownload();
  assert.equal(withdrawn.controller.getSnapshot().phase, 'idle');
  await withdrawn.controller.apply();
  assert.equal(withdrawn.calls.reload, 0);
});

test('native startup download can be applied without a manual check', async () => {
  const { controller, calls } = setup();
  await controller.apply();
  assert.equal(calls.reload, 0);
  await controller.apply(true);
  assert.equal(calls.reload, 1);
});
