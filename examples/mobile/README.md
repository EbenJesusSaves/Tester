# Mobile OTA integration examples

Read [the mobile guide](../../docs/mobile-client.md) for setup, exact server headers, signing, publishing, and device verification.

Copy these files into an **existing** Expo application:

| File | Purpose |
| --- | --- |
| `app.config.ts` | Validates app, channel, HTTPS manifest URL, certificate, and native runtime at build/export time |
| `ota-controller.ts` | Serializes checks/downloads/restarts, handles rollback, and bounds retry frequency |
| `useOtaUpdates.ts` | Observes native update state and triggers foreground checks with jitter |
| `UpdateBanner.tsx` | Offers a restart after saving user work and checking the application's safe-to-restart guard |

Keep the package versions chosen by `npx expo install expo-updates` in your own application. This directory's pinned Expo SDK 57 dependencies are a validation fixture, not an instruction to upgrade your app.

To validate the examples in this directory:

```bash
npm ci --ignore-scripts
npm run typecheck
npm test
```

This directory has no application entry point, native project, certificate, or deployment credentials. Typechecking and coordinator tests do not replace Android/iOS release-build testing. Mount the hook once near the app root and expose its return value through props/context. Normal checks happen after foregrounding; this is not a push subscription or a continuous polling timer.
