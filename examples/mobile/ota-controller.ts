import type * as Updates from 'expo-updates';

export type OtaPhase = 'idle' | 'checking' | 'downloading' | 'ready' | 'restarting' | 'error';
export type OtaState = Readonly<{ phase: OtaPhase; error?: Error }>;
type UpdatesPort = Pick<typeof Updates, 'checkForUpdateAsync' | 'fetchUpdateAsync' | 'reloadAsync'>;
type Options = { now?: () => number; random?: () => number };

const MIN_MANUAL_INTERVAL_MS = 30_000;
const AUTO_INTERVAL_MS = 15 * 60_000;
const MAX_AUTO_INTERVAL_MS = 60 * 60_000;

/** One instance per JavaScript runtime prevents concurrent native update operations. */
export function createOtaController(updates: UpdatesPort, options: Options = {}) {
  const now = options.now ?? Date.now;
  const random = options.random ?? Math.random;
  const subscribers = new Set<() => void>();
  let state: OtaState = { phase: 'idle' };
  let busy = false;
  let nextManualAt = 0;
  let nextAutomaticAt = 0;
  let consecutiveFailures = 0;

  function setState(next: OtaState) {
    state = next;
    subscribers.forEach(notify => notify());
  }

  return {
    getSnapshot: () => state,
    subscribe(notify: () => void) {
      subscribers.add(notify);
      return () => { subscribers.delete(notify); };
    },
    async checkAndDownload(reason: 'automatic' | 'manual' = 'manual'): Promise<void> {
      if (busy || state.phase === 'ready' || state.phase === 'restarting') return;
      const startedAt = now();
      if (startedAt < nextManualAt || (reason === 'automatic' && startedAt < nextAutomaticAt)) return;
      // Set the lock synchronously, before yielding, including for multiple hook consumers.
      busy = true;
      nextManualAt = startedAt + MIN_MANUAL_INTERVAL_MS;
      setState({ phase: 'checking' });
      try {
        const check = await updates.checkForUpdateAsync();
        // A rollback has isAvailable=false; dropping it here strands broken releases.
        if (check.isAvailable || check.isRollBackToEmbedded) {
          setState({ phase: 'downloading' });
          const fetched = await updates.fetchUpdateAsync();
          setState({ phase: fetched.isNew || fetched.isRollBackToEmbedded ? 'ready' : 'idle' });
        } else {
          setState({ phase: 'idle' });
        }
        consecutiveFailures = 0;
      } catch (error) {
        consecutiveFailures += 1;
        setState({ phase: 'error', error: error instanceof Error ? error : new Error(String(error)) });
      } finally {
        // Retry only on a later foreground or manual action. No background loop.
        const interval = Math.min(AUTO_INTERVAL_MS * 2 ** Math.min(consecutiveFailures, 2), MAX_AUTO_INTERVAL_MS);
        nextAutomaticAt = now() + interval + Math.floor(random() * 30_000);
        busy = false;
      }
    },
    async apply(nativeUpdatePending = false): Promise<void> {
      if (busy || state.phase === 'restarting' || (state.phase !== 'ready' && !nativeUpdatePending)) return;
      busy = true;
      setState({ phase: 'restarting' });
      try {
        // The caller must persist drafts before this: JavaScript may stop immediately.
        await updates.reloadAsync();
      } catch (error) {
        // Keep the downloaded update available for another user-approved attempt.
        setState({ phase: 'ready', error: error instanceof Error ? error : new Error(String(error)) });
      } finally {
        busy = false;
      }
    },
  };
}
