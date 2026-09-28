// k6 script: load a real published runtime; the no-update path is not comparable.
import http from 'k6/http';
import { check, sleep } from 'k6';

if (!__ENV.OTA_URL || !__ENV.OTA_APP_ID || !__ENV.OTA_RUNTIME) {
  throw new Error('Set OTA_URL, OTA_APP_ID and OTA_RUNTIME to an existing published release');
}
export const options = {
  vus: Number(__ENV.OTA_VUS || 20),
  duration: __ENV.OTA_DURATION || '60s',
  thresholds: {
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(95)<100', 'p(99)<250'],
    checks: ['rate>0.999'],
  },
};

export default function () {
  const response = http.get(`${__ENV.OTA_URL.replace(/\/$/, '')}/manifest`, {
    headers: {
      'expo-app-id': __ENV.OTA_APP_ID,
      'expo-channel-name': __ENV.OTA_CHANNEL || 'dev',
      'expo-platform': __ENV.OTA_PLATFORM || 'android',
      'expo-runtime-version': __ENV.OTA_RUNTIME,
      'expo-protocol-version': '1',
      'expo-expect-signature': 'sig, keyid="main", alg="rsa-v1_5-sha256"',
      'eas-client-id': `00000000-0000-4000-8000-${String(__VU).padStart(12, '0')}`,
    },
    tags: { name: 'signed_manifest' },
  });
  check(response, {
    'signed manifest returned': (r) => r.status === 200 && r.body.includes('expo-signature') && r.body.includes('launchAsset'),
    'manifest is not shared-cacheable': (r) => (r.headers['Cache-Control'] || '').includes('no-store'),
  });
  sleep(1);
}
