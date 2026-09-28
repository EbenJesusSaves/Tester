import { useState } from 'react';
import { Button, Text, View } from 'react-native';
import type { useOtaUpdates } from './useOtaUpdates';

type Props = {
  ota: ReturnType<typeof useOtaUpdates>;
  canRestart: boolean;
  saveDrafts: () => Promise<void>;
};

/** Place away from critical transaction flows; saveDrafts must finish before restart. */
export function UpdateBanner({ ota, canRestart, saveDrafts }: Props) {
  const [saving, setSaving] = useState(false);
  const [saveFailed, setSaveFailed] = useState(false);
  if (!ota.enabled) return null;

  async function restart() {
    if (!canRestart || saving || ota.busy) return;
    setSaving(true);
    setSaveFailed(false);
    try {
      await saveDrafts();
      await ota.applyUpdate();
    } catch {
      setSaveFailed(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <View accessibilityLiveRegion="polite">
      {ota.pending ? (
        <>
          <Text>An update is ready. Restart when you are ready to continue.</Text>
          <Button title="Restart to update" disabled={!canRestart || saving || ota.busy} onPress={() => void restart()} />
        </>
      ) : (
        <Button title={ota.busy ? 'Checking for updates…' : 'Check for updates'} disabled={ota.busy} onPress={() => void ota.checkNow()} />
      )}
      {ota.error ? <Text>Could not complete the update. You can keep using the app and try again later.</Text> : null}
      {saveFailed ? <Text>Could not save your work. Finish saving before restarting.</Text> : null}
    </View>
  );
}
