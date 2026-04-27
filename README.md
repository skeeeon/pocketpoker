# pocketpoker

A private friend-group poker app supporting Hold'em, Omaha, and six custom
variants. The server is a single Go binary that imports PocketBase as a
library; the frontend is a Vue 3 SPA served from the same process.

Built for trust-based play among friends — server is authoritative, no
real money, no anti-cheat surface beyond standard auth.

## Variants

All variants reduce to the constraint `hand_used + board_used = 5`,
encoded in [engine/variants.go](engine/variants.go).

| Variant      | Hole cards | From hand | From board | Max seats |
| ------------ | ---------- | --------- | ---------- | --------- |
| Hold'em      | 2          | 0–2       | 3–5        | 8         |
| Omaha        | 4          | exactly 2 | exactly 3  | 8         |
| KCK          | 3          | 0–3       | 2–5        | 8         |
| Kansas City  | 4          | 0–4       | 1–5        | 8         |
| Portland     | 5          | 1–4       | 1–4        | 8         |
| Miami        | 5          | 0–5       | 0–5        | 8         |
| Dubai        | 7          | 0–5       | 0–5        | 6         |
| Nova Scotia  | 7          | 0–2       | 3–5        | 6         |

Seat caps for 7-card variants come from `floor((52 − 5) / 7) = 6`.

## Stack

- **Backend**: Go 1.25, [PocketBase](https://pocketbase.io) v0.37 imported
  as a library (not the standalone binary). SQLite storage,
  `crypto/rand` shuffles, real goroutines.
- **Hand evaluation**: [chehsunliu/poker](https://github.com/chehsunliu/poker)
  (Cactus Kev port) wrapped by custom variant combo enumeration.
- **Frontend**: Vue 3 + Composition API + TypeScript, built with Vite.
  Uses the official PocketBase JS SDK for auth, REST, and SSE realtime.
- **Realtime**: PocketBase's built-in SSE subscriptions. No external
  message bus.

## Project layout

```
pocketpoker/
├── cmd/server/main.go         # PB entry point + migration registration
├── engine/                    # Pure game logic, zero PB dependency
│   ├── card.go                # Card / Suit / Rank, parsing, JSON
│   ├── deck.go                # Fisher-Yates shuffle, deal helpers
│   ├── variants.go            # 8-variant config including max_seats
│   ├── evaluator.go           # 5-card eval + variant combo enumeration
│   ├── state.go               # HandState + Phase / Action enums
│   ├── betting.go             # ApplyAction, round-completion logic
│   ├── pot.go                 # Single main pot (side pots TODO)
│   └── *_test.go
├── server/
│   ├── handlers.go            # All custom HTTP handlers
│   ├── store.go               # PB <-> engine state mapping w/ versioning
│   └── routes.go              # Route registration
├── pb_migrations/             # Go-based PocketBase migrations
├── pb_data/                   # PB data dir (gitignored)
└── web/                       # Vue 3 SPA
    ├── src/
    │   ├── composables/       # useAuth, useTable, usePlayerHand, useVariants
    │   ├── views/             # LoginView, LobbyView, TableView
    │   └── pb.ts              # PocketBase client singleton
    └── package.json
```

The `engine` package must not import PocketBase. Everything else stays
flat until a single file actually starts hurting.

## Running locally

### Prerequisites

- Go 1.25+
- Node 20+ and npm

### First-time setup

```bash
# Backend: pull deps and build the binary.
go mod download
go build -o server.exe ./cmd/server

# Frontend: install npm deps.
cd web && npm install
```

### Dev workflow

Open two terminals.

```bash
# Terminal 1 — Go server (runs PB on :8090, applies migrations on boot).
./server.exe serve

# Terminal 2 — Vite dev server with HMR (proxies API to :8090 in dev).
cd web && npm run dev
```

The Vite dev server typically lands on `http://localhost:5173`.
PocketBase's admin UI is at `http://localhost:8090/_/`.

### First-run admin

On a fresh `pb_data/`, the first user to hit the PB admin UI creates
the superuser account. From there, create accounts for the friend
group via Collections → users → New record. Onboarding is closed by
design — there is no public signup.

## Testing

```bash
# Engine unit tests (variant scenarios, betting edges, full-hand sims).
go test ./engine/...

# Frontend type-check + production build.
cd web && npm run build
```

## Architecture notes

- **Engine boundary is load-bearing.** All game rules live in `engine/`
  with no PB dependency. The server layer translates intents into
  `engine.ApplyAction` calls and persists the resulting `HandState`.
- **Server is authoritative.** Clients post intents (`fold`, `bet`,
  etc.); the server validates legality, applies the action, and writes
  back. Realtime subscriptions fan out the new state.
- **Optimistic concurrency.** Each `hands` row carries a `version`
  column that the server bumps on every write. Action submissions
  include the client's last-seen version; mismatches return HTTP 409
  and the client retries once after a refetch.
- **Privacy-critical reads.** The `hand_players` collection's API rule
  reveals opponents' hole cards only when `hand.phase` is `showdown`
  or `complete`. Because PocketBase realtime doesn't push records that
  *become* visible due to a parent-record rule re-evaluation, the
  client re-fetches `hand_players` on the phase transition into a
  reveal state (see [usePlayerHand.ts](web/src/composables/usePlayerHand.ts)).
- **No commit-reveal.** Trust model is the friend group; the server
  shuffles and deals.

## Custom HTTP endpoints

All under `/api/poker/*`, all auth-required.

| Method | Path                                  | Purpose                              |
| ------ | ------------------------------------- | ------------------------------------ |
| POST   | `/tables/{id}/sit`                    | Take an empty seat with a buy-in     |
| POST   | `/tables/{id}/leave`                  | Vacate seat (rejected mid-hand)      |
| POST   | `/tables/{id}/ready`                  | Mark caller ready for the next hand  |
| POST   | `/tables/{id}/start-hand`             | Dealer-only; deals a new hand        |
| POST   | `/hands/{id}/action`                  | Submit fold/check/call/bet/raise/all-in |
| POST   | `/hands/{id}/fold-player`             | Dealer-only; force-fold an AFK player |
| GET    | `/hands/{id}/replay`                  | Post-hand only; deck + action log    |

## Status

Implementation tracks a phased plan; through Phase 5 the app supports:

- All 8 variants with dealer-choice variant picker
- Seat-cap enforcement (engine + UI)
- Dealer rotation with a deterministic first-hand rule
- Per-seat ready-up between hands so players can review the winner
- Showdown UI highlighting each winner's chosen 5 cards (hole + board)
- Mobile-responsive layout

Known gaps (deferred until a real session demands them):

- Side pots — engine returns a clear error on multi-way unequal-stack
  all-ins; the failing test is `t.Skip("side pots: TODO")`-marked.
- Action timer / clock.
- Hand history / replay viewer (data is captured; no UI yet).
- Disconnect handling beyond the dealer's manual `fold-player` button.

## Out of scope

Real money, regulatory surface, public signup, anti-cheat, mobile
native apps, push notifications. This is a private friend-group app.
