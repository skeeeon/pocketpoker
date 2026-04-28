# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Production-style build: SPA first (go:embed pulls web/dist/ into the binary),
# then Go binary. Order matters — go build embeds whatever is in web/dist/ at
# compile time.
cd web && npm run build && cd ..
go build -o server.exe ./cmd/server

# Run the backend (PocketBase on :8090, applies migrations on boot,
# serves the embedded SPA at /).
./server.exe serve

# Frontend dev server with HMR (typically :5173). In dev, hit Vite directly;
# the SPA talks to PB on :8090 via the JS SDK with no proxy.
cd web && npm run dev

# Engine unit tests.
go test ./engine/...

# A single engine test by name.
go test ./engine/ -run TestFoo

# End-to-end HTTP integration test (requires a running server with seeded variants).
bash tests/integration.sh
```

There is no Go linter configured beyond `go vet` / `go build`. There is no
frontend test runner — `npm run build` (which runs `vue-tsc -b && vite build`)
is the only frontend check.

## High-level architecture

Single Go binary that imports PocketBase v0.37 **as a library** (not the
standalone PB binary) and embeds the built Vue 3 + TypeScript SPA via
`go:embed` (`web/embed.go` exports `web.Dist()`, mounted by
`server.RegisterRoutes` at `/{path...}` with index.html SPA fallback).
SQLite storage. SSE realtime via PocketBase's built-in subscriptions —
no external message bus.

`web/dist/.gitkeep` is committed (mirrored from `web/public/.gitkeep`,
which Vite copies into every build) so `go build` always has something
to embed even on a fresh clone before `npm run build`. Don't remove
either `.gitkeep` — `//go:embed all:dist` fails at compile time if
`web/dist/` is empty.

### Layered boundaries

- **`engine/`** — pure game logic. **Must not import PocketBase or any
  storage.** All transitions accept and return `engine.HandState`. The
  evaluator wraps `chehsunliu/poker` (Cactus Kev port) and enumerates
  variant-specific (hand, board) combos via `Variant.ValidCombos()`.
  All 10 variants reduce to `hand_used + board_used = 5`. Bot
  decision-making (`bot.go`) lives here too and stays pure — the
  server-side `bot_loop.go` is what drives bot turns asynchronously.
- **`server/`** — HTTP handlers + the **only** PB↔engine adapter
  (`store.go`). Translates client intents into `engine.ApplyAction` /
  `engine.Deal` calls and persists the resulting `HandState`.
- **`pb_migrations/`** — Go-based PB migrations registered via blank
  import in `cmd/server/main.go`. Automigrate is **only** enabled when
  the binary is run via `go run` (detected by `os.TempDir()` prefix on
  `os.Args[0]`); deployed binaries never auto-write migrations.
- **`web/`** — Vue 3 SPA using the official PocketBase JS SDK directly
  for auth, REST, and realtime. There is no separate API client layer.
  Also a Go package: `web/embed.go` exposes the built `dist/` as an
  `fs.FS` for the server to mount. The static route is registered
  **last** in `RegisterRoutes` so the more-specific `/api/*` and `/_/*`
  matches keep priority over the SPA catch-all.

### Authority and concurrency model

- **Server is authoritative.** Clients post intents (`fold`, `bet`,
  `call`, etc.); the server validates legality, calls the engine, and
  writes back inside a transaction.
- **Optimistic concurrency on `hands.version`.** Every action submission
  echoes the client's last-seen version. `SaveHand` checks the version
  inside `RunInTransaction` and returns `ErrVersionMismatch` (mapped to
  HTTP 409) on conflict. Clients refetch and retry once.
- **Stack accounting is split** between two collections to avoid
  double-bookkeeping: `seats.stack` is only persisted at hand start and
  at hand completion. Mid-hand, `LoadHand` derives the running stack as
  `seats.stack - sum(actions[seat].amount)`. Don't write `seats.stack`
  mid-hand — it's the canonical "between hands" value.

### Privacy-critical reads

- The `hand_players` collection's API rule reveals opponents' hole cards
  only when `hand.phase == "showdown"` **or** the row is the user's own.
  Phase `complete` does **not** match the showdown clause — see the long
  note in `tests/integration.sh`. If this needs to change, widen the
  rule rather than holding state in memory.
- **PB realtime does not push records that *become* visible** when a
  parent record's API rule re-evaluates. So when the hand transitions
  into a reveal phase, the client must re-fetch `hand_players`. See
  `web/src/composables/usePlayerHand.ts` — preserve that re-fetch.
- `hands.deck_state` is hidden from list/view rules and only exposed
  via the post-hand `/api/poker/hands/{id}/replay` endpoint.

### Wire format quirks

- `engine.Phase` has custom `MarshalJSON` / `UnmarshalJSON` so the wire
  shape is the lowercase string label, matching the SelectField on
  `hands.phase`. The unmarshaler accepts the legacy integer encoding
  too, so older persisted rows still load. If you add a new phase, add
  the label to `phaseStrings` *and* `server.phaseFromString`.
- `unmarshalAny` in `server/store.go` exists because PB returns JSON
  columns as either `string` or `[]byte` depending on the path. Use it
  for any new JSON-typed PB column reads.

### Game-rule constraints worth knowing before editing

- `engine.MinPlayers = 3`. v1 deliberately does not implement heads-up
  ordering — `Deal` rejects fewer than 3 players.
- Variant `MaxSeats` for 7-card variants (Dubai, Nova Scotia) is 6,
  derived from `floor((52 − 5) / 7)`. Don't lift the cap without
  changing the deal logic.
- **Side pots are implemented in `engine/pot.go`.** `buildPots`
  constructs the main pot plus any side pots from the action log by
  sweeping distinct total-commitment levels of non-folded seats; each
  pot's `Eligible` list is the non-folded seats that reached that
  level. `goToShowdown` evaluates each non-folded hand once and awards
  every pot independently. The legacy `state.Winners` field is still
  populated (sum-per-seat across pots) for backward compatibility; the
  new per-pot breakdown lives in `state.Pots`.
- Action ordering: preflop starts at the seat after the BB; postflop
  starts at the first active seat clockwise from the dealer. `Deal`
  posts blinds via the same `Action` log path as normal play, so
  `Actions` is a uniform replayable log.
