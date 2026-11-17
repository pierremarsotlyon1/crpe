# Mobile application

- Expo/React Native. Entry point: `index.ts`; initial screen: `src/App.tsx`; API origin: `src/api.ts`.
- `EXPO_PUBLIC_API_URL` must be reachable from the device (a physical device cannot use the developer machine's localhost). It is public configuration; never put secrets in `EXPO_PUBLIC_*`.
- Use `@crpe/api-client`. Keep generated API types in the shared package, not duplicated here.
- The current screen is a foundation only. Auth, practice, notifications, photo upload and offline synchronization are pending.
- Never add checkout, purchase links or a billing flow. UI is French; code is English.
- Use the Expo SDK-compatible React/React Native versions together; check official docs before upgrading.
- Root commands: `npm run dev -w @crpe/mobile`, `npm run typecheck -w @crpe/mobile`, `npm run lint -w @crpe/mobile`, `npm run build -w @crpe/mobile`.
- `build` exports iOS/Android JavaScript bundles. It does not build/sign native binaries. `ios`/`android` require local native SDKs and are separate verification steps.
- Generated `.expo/`, `dist/`, `ios/`, `android/` are not normal authoring context.
