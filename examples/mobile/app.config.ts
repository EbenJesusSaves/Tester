import type { ConfigContext, ExpoConfig } from 'expo/config';
import { X509Certificate } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function required(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`Missing ${name}; supply the same environment for build and publish.`);
  return value;
}

export default ({ config }: ConfigContext): ExpoConfig => {
  // Never silently turn an unconfigured build or export into a production release.
  const channel = required('OTA_CHANNEL');
  if (!['dev', 'uat', 'prod'].includes(channel)) {
    throw new Error('OTA_CHANNEL must be dev, uat, or prod.');
  }
  const appId = required('OTA_APP_ID');
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(appId)) {
    throw new Error('OTA_APP_ID must be the UUID from the xprem dashboard.');
  }
  const manifestUrl = new URL(required('OTA_MANIFEST_URL'));
  if (manifestUrl.protocol !== 'https:' || manifestUrl.username || manifestUrl.password ||
      manifestUrl.search || manifestUrl.hash || !manifestUrl.pathname.endsWith('/manifest')) {
    throw new Error('OTA_MANIFEST_URL must be an HTTPS URL ending in /manifest, without credentials/query/hash.');
  }
  const certificatePath = required('OTA_CERTIFICATE_PATH');
  if (!existsSync(resolve(certificatePath))) {
    throw new Error('OTA_CERTIFICATE_PATH must point to the public signing certificate for OTA_APP_ID.');
  }
  const certificate = new X509Certificate(readFileSync(resolve(certificatePath)));
  if (Date.now() < Date.parse(certificate.validFrom) || Date.now() >= Date.parse(certificate.validTo)) {
    throw new Error('The OTA signing certificate is not currently valid.');
  }
  if (certificate.publicKey.asymmetricKeyType !== 'rsa' ||
      (certificate.publicKey.asymmetricKeyDetails?.modulusLength ?? 0) < 2048) {
    throw new Error('The OTA signing certificate must use an RSA key of at least 2048 bits.');
  }
  if (config.ios?.runtimeVersion || config.android?.runtimeVersion) {
    throw new Error('Remove platform runtimeVersion overrides when using this explicit OTA_RUNTIME_VERSION example.');
  }
  if (!config.name || !config.slug) throw new Error('Keep name and slug in your existing app.json.');

  return {
    ...config,
    name: config.name,
    slug: config.slug,
    // The example chooses explicit native compatibility IDs. Change this for any
    // native dependency, SDK, permission, signing certificate, or native config change.
    runtimeVersion: required('OTA_RUNTIME_VERSION'),
    updates: {
      enabled: true,
      url: manifestUrl.toString(),
      checkAutomatically: 'ON_ERROR_RECOVERY',
      fallbackToCacheTimeout: 0,
      useEmbeddedUpdate: true,
      disableAntiBrickingMeasures: false,
      codeSigningCertificate: certificatePath,
      codeSigningMetadata: { keyid: 'main', alg: 'rsa-v1_5-sha256' },
      requestHeaders: {
        'expo-app-id': appId,
        'expo-channel-name': channel,
      },
    },
  };
};
