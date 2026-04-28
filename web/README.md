# pocketpoker SPA

Vue 3 + TypeScript + Vite frontend for [pocketpoker](../README.md). The
built `dist/` is embedded into the Go binary via `go:embed` (see
[embed.go](embed.go)) and served at `/` in production. In dev, run Vite
on its own port and let it talk to the PocketBase backend on `:8090`
through the official PB JS SDK — there is no proxy or separate API
client layer.

## Commands

```bash
npm install        # first-time deps
npm run dev        # Vite dev server with HMR (typically :5173)
npm run build      # vue-tsc -b && vite build → dist/
```

`npm run build` is the only frontend check; there is no test runner.
The build runs `vue-tsc` first, so type errors fail the build.

## Layout

- `src/composables/` — `useAuth`, `useTable`, `usePlayerHand`, `useVariants`
- `src/views/` — `LoginView`, `LobbyView`, `TableView`
- `src/pb.ts` — PocketBase client singleton

## Notes

- `dist/.gitkeep` is committed (mirrored from `public/.gitkeep`, which
  Vite copies into every build) so `go build` always has something to
  embed even on a fresh clone before `npm run build`.
- Realtime: PB SSE subscriptions handle live updates. When `hand.phase`
  transitions into a reveal state, `usePlayerHand` re-fetches
  `hand_players` because PB realtime does not push records that *become*
  visible due to a parent record's API rule re-evaluating — see the
  comment in [usePlayerHand.ts](src/composables/usePlayerHand.ts).
