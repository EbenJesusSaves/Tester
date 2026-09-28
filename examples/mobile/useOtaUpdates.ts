import { useCallback, useEffect, useSyncExternalStore } from 'react';
import { AppState, Platform } from 'react-native';
import * as Updates from 'expo-updates';
import { createOtaController } from './ota-controller';

const controller = createOtaController(Updates);

/** Mount one owner near the root; expose its return value through your app context. */
export function useOtaUpdates() {
  // This listens to LOCAL native download/check events. It is not a server push subscription.
  const native = Updates.useUpdates();
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const enabled = Platform.OS !== 'web' && !__DEV__ && Updates.isEnabled;
  const nativeBusy = native.isStartupProcedureRunning || native.isChecking || native.isDownloading || native.isRestarting;
  const pending = native.isUpdatePending || state.phase === 'ready';

  const checkNow = useCallback(async () => {
    if (enabled && !nativeBusy && !pending && AppState.currentState === 'active') {
      await controller.checkAndDownload('manual');
    }
  }, [enabled, nativeBusy, pending]);

  useEffect(() => {
    if (!enabled || nativeBusy || pending) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const clearTimer = () => { if (timer !== undefined) clearTimeout(timer); timer = undefined; };
    const schedule = () => {
      clearTimer();
      // Spread synchronized launches across 10 seconds; this does not delay app startup.
      timer = setTimeout(() => {
        if (AppState.currentState === 'active') void controller.checkAndDownload('automatic');
      }, Math.floor(Math.random() * 10_000));
    };
    if (AppState.currentState === 'active') schedule();
    const subscription = AppState.addEventListener('change', next => {
      if (next === 'active') schedule(); else clearTimer();
    });
    return () => { clearTimer(); subscription.remove(); };
  }, [enabled, nativeBusy, pending]);

  const applyUpdate = useCallback(async () => {
    if (enabled && !nativeBusy && pending && AppState.currentState === 'active') {
      await controller.apply(native.isUpdatePending);
    }
  }, [enabled, nativeBusy, pending, native.isUpdatePending]);

  return {
    enabled,
    pending,
    busy: nativeBusy || ['checking', 'downloading', 'restarting'].includes(state.phase),
    phase: state.phase,
    error: state.error ?? native.checkError ?? native.downloadError,
    progress: native.downloadProgress,
    currentlyRunning: native.currentlyRunning,
    checkNow,
    applyUpdate,
  };
}
