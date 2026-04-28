<script setup lang="ts">
import { computed, ref, toRef, watch, watchEffect } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '../pb'
import { useAuth } from '../composables/useAuth'
import { useTable, BOT_PERSONALITIES } from '../composables/useTable'
import { usePlayerHand } from '../composables/usePlayerHand'
import { useVariants, variantRuleSummary, type Variant } from '../composables/useVariants'
import UserAvatar from '../components/UserAvatar.vue'

const props = defineProps<{ id: string }>()
const router = useRouter()
const { user } = useAuth()
const tableId = toRef(props, 'id')
const { table, seats, hand, userBySeat, error: tableError } = useTable(tableId)
const handId = computed(() => hand.value?.id ?? '')
const { allHands, reload: reloadPlayerHand } = usePlayerHand(handId)

// PB realtime doesn't push records that *become* visible due to a
// parent-record rule re-evaluation. When hand.phase flips into a state
// that reveals opponents' hole cards (showdown / complete), re-fetch
// hand_players so they appear without requiring a page reload.
watch(
  () => hand.value?.phase,
  (next, prev) => {
    const reveal = next === 'showdown' || next === 'complete'
    const wasReveal = prev === 'showdown' || prev === 'complete'
    if (reveal && !wasReveal) reloadPlayerHand()
  },
)
const { variants, byId } = useVariants()

const error = ref<string | null>(null)
const busy = ref(false)

const currentVariant = computed<Variant | null>(() => {
  if (!hand.value) return null
  return byId.value[hand.value.variant] ?? null
})

// Hole card count to render for opponents (face-down) — defaults to 2
// before the first hand of an unknown variant has loaded.
const holeCardCount = computed(() => currentVariant.value?.hand_size ?? 2)

const mySeat = computed(() =>
  seats.value.find((s) => s.user === user.value?.id) ?? null,
)

const isSeated = computed(() => mySeat.value !== null)

const myRoundCommit = computed(() => {
  if (!hand.value || !mySeat.value) return 0
  const phase = hand.value.phase
  return (hand.value.actions ?? [])
    .filter((a) => a.seat === mySeat.value!.seat_number && a.phase === phase)
    .reduce((sum, a) => sum + a.amount, 0)
})

const toCall = computed(() => {
  if (!hand.value) return 0
  return Math.max(0, hand.value.current_bet - myRoundCommit.value)
})

const isMyTurn = computed(() => {
  if (!hand.value || !mySeat.value) return false
  if (hand.value.phase === 'complete' || hand.value.phase === 'showdown') return false
  return hand.value.current_actor_seat === mySeat.value.seat_number
})

const isHandComplete = computed(() => hand.value?.phase === 'complete')

// Bet vs raise: a "bet" is only legal when no one has put chips in this
// round. The blinds count — for the BB facing an unraised pot,
// current_bet > 0 even though toCall === 0, so the action is "raise"
// (or "check"), never "bet".
const currentBet = computed(() => hand.value?.current_bet ?? 0)
const minRaiseTotal = computed(() => {
  if (!table.value) return 0
  return currentBet.value + table.value.big_blind
})

// Only seats that were in the previous hand (have a hand_players row)
// need to ready up. Fresh sit-downs after the hand completed skip the
// gate — they have nothing to review.
const seatsRequiringReady = computed(() => {
  const prevHandSeatIds = new Set(allHands.value.map((h) => h.seat))
  return seats.value.filter((s) => prevHandSeatIds.has(s.id))
})
const everyoneReady = computed(() =>
  seatsRequiringReady.value.every((s) => s.ready_for_next),
)
const notReadySeats = computed(() =>
  seatsRequiringReady.value.filter((s) => !s.ready_for_next).map((s) => s.seat_number),
)
const myReady = computed(() => mySeat.value?.ready_for_next ?? false)

// Compute the seat that will deal the NEXT hand, mirroring server logic
// in handlers.go (nextActiveSeatClockwise + the first-hand branch).
// Bots are excluded from dealer rotation — they can't drive start-hand,
// so the button always lands on a human. First hand: lowest active
// human seat. Otherwise: next eligible clockwise from the previous
// dealer.
const nextDealerSeat = computed<number | null>(() => {
  const eligible = seats.value
    .filter((s) => s.status === 'active' && !s.bot_personality)
    .sort((a, b) => a.seat_number - b.seat_number)
  if (eligible.length === 0) return null
  if (!hand.value) {
    return eligible[0].seat_number
  }
  const prev = table.value?.current_dealer_seat ?? -1
  const idx = eligible.findIndex((s) => s.seat_number === prev)
  if (idx < 0) return eligible[0].seat_number
  return eligible[(idx + 1) % eligible.length].seat_number
})
const isDealerMe = computed(() => {
  if (!mySeat.value || nextDealerSeat.value === null) return false
  return mySeat.value.seat_number === nextDealerSeat.value
})

// Sitting down. -1 sentinel means "not yet picked"; the watcher below
// fills it from `nextOpenSeat()` once table+seats are loaded.
const sitSeatNumber = ref<number>(-1)
const sitBuyIn = ref<number>(0)
function nextOpenSeat(): number {
  if (!table.value) return -1
  const taken = new Set(seats.value.map((s) => s.seat_number))
  for (let i = 0; i < table.value.max_seats; i++) {
    if (!taken.has(i)) return i
  }
  return -1
}
async function sitDown() {
  if (!table.value) return
  busy.value = true
  error.value = null
  try {
    const seat = sitSeatNumber.value
    const buy = sitBuyIn.value || table.value.buy_in
    await pb.send(`/api/poker/tables/${table.value.id}/sit`, {
      method: 'POST',
      body: { seat_number: seat, buy_in_amount: buy },
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

// Bot picker state. Opened by the "Add Bot" button when a human at the
// table wants to populate an empty seat. Personality keys mirror the
// engine's Personalities map.
const botPickerOpen = ref(false)
const botPersonalityKey = ref<string>('tight')
const botSeatNumber = ref<number>(-1)
const personalityOptions = Object.entries(BOT_PERSONALITIES) as [string, string][]

function openBotPicker() {
  botSeatNumber.value = nextOpenSeat()
  botPickerOpen.value = true
}

async function addBot() {
  if (!table.value) return
  if (botSeatNumber.value < 0) {
    error.value = 'no open seats'
    return
  }
  if (!BOT_PERSONALITIES[botPersonalityKey.value]) {
    error.value = `unknown personality ${botPersonalityKey.value}`
    return
  }
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/add-bot`, {
      method: 'POST',
      body: {
        seat_number: botSeatNumber.value,
        buy_in_amount: table.value.buy_in,
        personality: botPersonalityKey.value,
      },
    })
    botPickerOpen.value = false
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function removeBot(seatNumber: number) {
  if (!table.value) return
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/remove-bot`, {
      method: 'POST',
      body: { seat_number: seatNumber },
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function isBotSeat(seatNum: number): boolean {
  const s = seats.value.find((x) => x.seat_number === seatNum)
  return !!s?.bot_personality
}

// Add Bot is offered to any seated human while there's an open seat
// and no live hand running. The server enforces these too — this is
// just UI gating.
const canAddBot = computed(() => {
  if (!isSeated.value || !table.value) return false
  if (seats.value.length >= table.value.max_seats) return false
  if (hand.value && !isHandComplete.value) return false
  return true
})

// Surface the engine.MinPlayers=3 rule as a friendly nudge when the
// table is short. Only show to seated humans (otherwise they'd see it
// before sitting down, which is confusing).
const needsMorePlayers = computed(
  () => isSeated.value && seats.value.length < 3,
)

async function leaveTable() {
  if (!table.value) return
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/leave`, { method: 'POST' })
    router.push({ name: 'lobby' })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

// Variant picker
const pickerOpen = ref(false)
const pickedVariantKey = ref<string>('holdem')

function openPicker() {
  // Default to a variant that fits the current seat count when possible.
  const fits = variants.value.find((v) => seats.value.length <= v.max_seats)
  pickedVariantKey.value = fits?.key ?? variants.value[0]?.key ?? 'holdem'
  pickerOpen.value = true
}

function variantFits(v: Variant): boolean {
  return seats.value.length <= v.max_seats
}

function variantTooltip(v: Variant): string {
  const summary = variantRuleSummary(v)
  if (!variantFits(v)) {
    return `${summary}\n\nDisabled: ${seats.value.length} seated > ${v.max_seats} max.`
  }
  return summary
}

async function startHand() {
  if (!table.value) return
  const v = variants.value.find((x) => x.key === pickedVariantKey.value)
  if (!v) {
    error.value = `unknown variant ${pickedVariantKey.value}`
    return
  }
  if (!variantFits(v)) {
    error.value = `${v.name} supports at most ${v.max_seats} seats; ${seats.value.length} are seated.`
    return
  }
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/start-hand`, {
      method: 'POST',
      body: { variant_key: pickedVariantKey.value },
    })
    pickerOpen.value = false
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

const betAmount = ref<number>(0)

// Reset bet/raise input default whenever it's our turn so the input is
// pre-populated with the minimum legal value for the current state.
watchEffect(() => {
  if (!isMyTurn.value || !table.value) return
  if (currentBet.value === 0) {
    betAmount.value = table.value.big_blind
  } else {
    betAmount.value = minRaiseTotal.value
  }
})

async function setReady(ready: boolean) {
  if (!table.value) return
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/ready`, {
      method: 'POST',
      body: { ready },
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function submitAction(actionType: 'fold' | 'check' | 'call' | 'bet' | 'raise' | 'all_in', amount = 0) {
  if (!hand.value) return
  busy.value = true
  error.value = null
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      await pb.send(`/api/poker/hands/${hand.value.id}/action`, {
        method: 'POST',
        body: {
          action_type: actionType,
          amount,
          version: hand.value.version,
        },
      })
      busy.value = false
      return
    } catch (e: unknown) {
      const err = e as { status?: number; message?: string }
      if (err?.status === 409 && attempt === 0) {
        // refetch and retry once
        try {
          hand.value = (await pb.collection('hands').getOne(hand.value.id)) as never
        } catch {
          /* ignore */
        }
        continue
      }
      error.value = err?.message || String(e)
      busy.value = false
      return
    }
  }
  busy.value = false
}

// Display helpers
const suitGlyphs: Record<string, string> = { c: '♣', d: '♦', h: '♥', s: '♠' }
function formatCard(c: string): string {
  if (c.length !== 2) return c
  return c[0] + (suitGlyphs[c[1]] ?? c[1])
}
function suitClass(c: string): string {
  if (c.length !== 2) return ''
  if (c[1] === 'h' || c[1] === 'd') return 'red'
  return 'black'
}

function findHoleCardsForSeat(seatNum: number): string[] {
  const seatRec = seats.value.find((s) => s.seat_number === seatNum)
  if (!seatRec) return []
  const hp = allHands.value.find((h) => h.seat === seatRec.id)
  return hp?.hole_cards ?? []
}

// Mid-hand stack display. seats.stack is intentionally only written at
// hand start and hand completion (see CLAUDE.md), so during a live hand
// it shows the start-of-hand value. Derive the running stack the same
// way LoadHand does server-side: subtract this seat's total commits in
// the active hand. When no hand is live or the hand is complete, the
// seat record already holds the canonical value.
function runningStack(seatNum: number): number {
  const s = seats.value.find((x) => x.seat_number === seatNum)
  if (!s) return 0
  if (!hand.value || hand.value.phase === 'complete') return s.stack
  const committed = (hand.value.actions ?? [])
    .filter((a) => a.seat === seatNum)
    .reduce((sum, a) => sum + a.amount, 0)
  return s.stack - committed
}

function statusForSeat(seatNum: number): string {
  const seatRec = seats.value.find((s) => s.seat_number === seatNum)
  if (!seatRec) return ''
  const hp = allHands.value.find((h) => h.seat === seatRec.id)
  return hp?.status ?? ''
}

function isDealer(seatNum: number): boolean {
  return hand.value ? hand.value.dealer_seat === seatNum : false
}

function isCurrentActor(seatNum: number): boolean {
  return hand.value ? hand.value.current_actor_seat === seatNum && !isHandComplete.value : false
}

// Whether this seat belongs to the signed-in user. Drives the "you"
// styling on the seat tile and is_me-based affordances.
function seatIsMine(seatNum: number): boolean {
  const u = userBySeat.value[seatNum]
  return !!u && user.value?.id === u.id
}

// Whether this seat will deal the next hand. Used to surface a hint
// between hands so players know who they're waiting on without
// reading the controls strip.
function isNextDealer(seatNum: number): boolean {
  if (hand.value && !isHandComplete.value) return false
  return nextDealerSeat.value === seatNum
}

function winnerInfo(seatNum: number) {
  if (!hand.value?.winner_seats) return null
  return hand.value.winner_seats.find((w) => w.seat === seatNum) ?? null
}

// Set of card strings used by ANY winner. Used to highlight community
// cards that are part of a winning 5. Each winner's own hole-card
// highlight is restricted to that winner's chosen cards.
//
// Note: SeatResult.cards is null for "won uncontested" finishes (the
// engine doesn't reveal cards when everyone folds), so guard the iter.
const winningCommunityCards = computed<Set<string>>(() => {
  const set = new Set<string>()
  if (!hand.value?.winner_seats) return set
  const board = new Set(hand.value.community_cards ?? [])
  for (const w of hand.value.winner_seats) {
    if (!w.cards) continue
    for (const c of w.cards) {
      if (board.has(c)) set.add(c)
    }
  }
  return set
})

function isCardChosenForSeat(seatNum: number, card: string): boolean {
  const w = winnerInfo(seatNum)
  if (!w || !w.cards) return false
  return w.cards.includes(card)
}

// Plain display name for a seat — no "(you)" suffix because the seat
// tile gets its own visual self-indicator. Falls back to email, then
// to a short id slice, then to the literal "seat N" placeholder.
function seatName(seatNum: number): string {
  const u = userBySeat.value[seatNum]
  if (!u) return `seat ${seatNum}`
  return u.name?.trim() || u.email || u.id.slice(0, 6)
}

// Used by control-strip prose ("waiting for X to deal"), so the
// "(you)" suffix is still useful here for self-referencing copy.
function seatLabel(seatNum: number): string {
  const u = userBySeat.value[seatNum]
  if (!u) return `seat ${seatNum}`
  const me = user.value?.id === u.id
  return `${seatName(seatNum)}${me ? ' (you)' : ''}`
}

// Seat numbers rendered in the opponents ribbon. When seated, we
// exclude our own seat — the me-dock takes over its display. When not
// seated (spectator), every seat renders here so the full table is
// visible without a me-dock.
const opponentSeatNumbers = computed<number[]>(() => {
  if (!table.value) return []
  const all = Array.from({ length: table.value.max_seats }, (_, i) => i)
  if (!isSeated.value) return all
  return all.filter((n) => !seatIsMine(n))
})

// Init / auto-correct sit-down defaults whenever table or seats change.
// Watching both refs (instead of just `table`) ensures the seat picker
// fills in once seats finish loading — `useTable` fetches table first
// and seats second, so a single-ref watcher could sample stale seats.
//
// The seat is only rewritten when the current value is out of range or
// already taken, so a deliberate user override is preserved (until
// someone else snipes it, in which case auto-correct kicks in).
watch(
  [table, seats],
  () => {
    const t = table.value
    if (t && sitBuyIn.value === 0) sitBuyIn.value = t.buy_in

    const cur = sitSeatNumber.value
    const inRange = !!t && cur >= 0 && cur < t.max_seats
    const taken = seats.value.some((s) => s.seat_number === cur)
    if (!inRange || taken) {
      const next = nextOpenSeat()
      if (next >= 0) sitSeatNumber.value = next
    }
  },
  { immediate: true },
)
</script>

<template>
  <section class="table-view">
    <p v-if="tableError" class="err">{{ tableError }}</p>
    <p v-if="!table">loading…</p>
    <template v-else>
      <header>
        <h2>{{ table.name }}</h2>
        <span class="meta">
          SB/BB {{ table.small_blind }}/{{ table.big_blind }} · buy-in {{ table.buy_in }}
        </span>
        <span
          v-if="hand && currentVariant"
          class="variant-tag"
          :title="variantRuleSummary(currentVariant)"
        >
          {{ currentVariant.name }}
          <span class="qmark" aria-label="variant rules">?</span>
        </span>
        <span v-if="hand" class="phase">{{ hand.phase }}</span>
      </header>

      <!-- Opponents grid: wrapping uniform-width tiles. Each tile is the
           same column width (no per-tile shrink/grow), names truncate
           with ellipsis, and the remove-bot button takes inline space
           instead of overlaying the avatar. -->
      <ol class="opponents">
        <li
          v-for="n in opponentSeatNumbers"
          :key="n"
          class="opponent"
          :class="{
            empty: !seats.some((s) => s.seat_number === n),
            'is-current': isCurrentActor(n),
            folded: statusForSeat(n) === 'folded',
          }"
        >
          <template v-if="seats.find((s) => s.seat_number === n)">
            <div class="opp-head">
              <UserAvatar :user="userBySeat[n]" :size="22" />
              <span class="opp-name" :title="seatName(n)">{{ seatName(n) }}</span>
              <button
                v-if="isBotSeat(n) && isSeated && (!hand || isHandComplete)"
                class="remove-bot-btn"
                :disabled="busy"
                :title="`remove ${seatName(n)}`"
                @click="removeBot(n)"
              >×</button>
            </div>
            <div class="opp-meta">
              <span class="opp-stack" :title="`stack ${runningStack(n)}`">{{ runningStack(n) }}</span>
              <span class="opp-badges">
                <span v-if="isDealer(n)" class="badge dealer" title="dealer">D</span>
                <span v-else-if="isNextDealer(n)" class="badge dealer next" title="deals next">D</span>
                <span v-if="hand && hand.small_blind_seat === n" class="badge sb" title="small blind">SB</span>
                <span v-if="hand && hand.big_blind_seat === n" class="badge bb" title="big blind">BB</span>
                <span v-if="isCurrentActor(n)" class="badge actor" title="to act">▶</span>
                <span
                  v-if="isHandComplete && seats.find((s) => s.seat_number === n)?.ready_for_next"
                  class="badge ready"
                  title="ready"
                >✓</span>
              </span>
            </div>
            <!-- Hole cards only when actually revealed (showdown/complete).
                 Face-down "?? ??" fillers add no information mid-hand and
                 just bloat the tile, so we omit them. -->
            <div class="opp-cards" v-if="findHoleCardsForSeat(n).length">
              <span
                v-for="c in findHoleCardsForSeat(n)"
                :key="c"
                class="card mini"
                :class="[suitClass(c), { chosen: isCardChosenForSeat(n, c) }]"
              >
                {{ formatCard(c) }}
              </span>
            </div>
            <div v-if="statusForSeat(n) === 'folded'" class="opp-tag">folded</div>
            <div v-if="winnerInfo(n)" class="opp-winner" :title="winnerInfo(n)!.class">
              +{{ winnerInfo(n)!.amount }}
            </div>
          </template>
          <template v-else>
            <div class="empty-label">
              <span class="seat-num">#{{ n }}</span>
              <span class="muted">empty</span>
            </div>
          </template>
        </li>
      </ol>

      <!-- Board centerpiece. Flex-grows to fill remaining vertical space
           between the opponents ribbon and the me-dock so the cards stay
           the visual focal point. -->
      <div class="board">
        <div class="board-cards">
          <span v-if="!hand?.community_cards?.length" class="muted">— no board yet —</span>
          <span
            v-for="c in hand?.community_cards ?? []"
            :key="c"
            class="card big"
            :class="[suitClass(c), { chosen: winningCommunityCards.has(c) }]"
          >
            {{ formatCard(c) }}
          </span>
        </div>
        <div v-if="hand" class="pot-display">pot <strong>{{ hand.pot }}</strong></div>
      </div>

      <!-- Me-dock: the user's own seat, larger and persistent above the
           action panel so hole cards + stack are always visible. Only
           rendered when the user is seated. -->
      <div
        v-if="mySeat"
        class="me-dock"
        :class="{ 'is-current': isMyTurn, folded: statusForSeat(mySeat.seat_number) === 'folded' }"
      >
        <div class="dock-head">
          <UserAvatar :user="userBySeat[mySeat.seat_number]" :size="34" />
          <div class="dock-id">
            <span class="dock-name">{{ seatName(mySeat.seat_number) }}</span>
            <span class="seat-num">#{{ mySeat.seat_number }} · you</span>
          </div>
          <div class="dock-badges">
            <span v-if="isDealer(mySeat.seat_number)" class="badge dealer" title="dealer">D</span>
            <span v-else-if="isNextDealer(mySeat.seat_number)" class="badge dealer next" title="deals next">D</span>
            <span v-if="hand && hand.small_blind_seat === mySeat.seat_number" class="badge sb" title="small blind">SB</span>
            <span v-if="hand && hand.big_blind_seat === mySeat.seat_number" class="badge bb" title="big blind">BB</span>
            <span v-if="isCurrentActor(mySeat.seat_number)" class="badge actor" title="to act">▶ acting</span>
          </div>
          <div class="dock-stack">stack <strong>{{ runningStack(mySeat.seat_number) }}</strong></div>
        </div>
        <div class="dock-cards" v-if="hand">
          <template v-if="findHoleCardsForSeat(mySeat.seat_number).length">
            <span
              v-for="c in findHoleCardsForSeat(mySeat.seat_number)"
              :key="c"
              class="card big"
              :class="[suitClass(c), { chosen: isCardChosenForSeat(mySeat.seat_number, c) }]"
            >
              {{ formatCard(c) }}
            </span>
          </template>
          <template v-else>
            <span class="card big hidden" v-for="m in holeCardCount" :key="m">??</span>
          </template>
        </div>
        <div v-if="winnerInfo(mySeat.seat_number)" class="dock-winner">
          won {{ winnerInfo(mySeat.seat_number)!.amount }} · {{ winnerInfo(mySeat.seat_number)!.class }}
        </div>
      </div>

      <!-- Sit / Leave / Start hand controls -->
      <div class="controls">
        <template v-if="!isSeated">
          <fieldset>
            <legend>Sit down</legend>
            <label>seat <input v-model.number="sitSeatNumber" type="number" min="0" /></label>
            <label>buy-in <input v-model.number="sitBuyIn" type="number" /></label>
            <button :disabled="busy" @click="sitDown">sit</button>
          </fieldset>
        </template>
        <template v-else>
          <button :disabled="busy" @click="leaveTable">leave table</button>
          <button
            v-if="canAddBot"
            :disabled="busy"
            class="add-bot-btn"
            @click="openBotPicker"
          >
            + add bot
          </button>
          <template v-if="!hand || isHandComplete">
            <button
              v-if="isDealerMe"
              :disabled="busy || seats.length < 3 || (isHandComplete && !everyoneReady)"
              :title="isHandComplete && !everyoneReady ? `waiting on seats ${notReadySeats.join(', ')} to ready` : ''"
              @click="openPicker"
            >
              start hand…
            </button>
            <span v-else-if="nextDealerSeat !== null" class="muted dealer-wait">
              waiting for {{ seatLabel(nextDealerSeat) }} to deal
            </span>
          </template>
          <span v-if="needsMorePlayers" class="hint">
            Need 3+ players to deal — add a bot?
          </span>
        </template>
      </div>

      <!-- Bot picker modal -->
      <div v-if="botPickerOpen" class="modal-backdrop" @click.self="botPickerOpen = false">
        <div class="modal" role="dialog" aria-label="Add bot">
          <h3>Add a bot</h3>
          <p class="muted">Each archetype plays a noticeably different style.</p>
          <fieldset class="bot-seat-pick">
            <label>
              seat
              <input
                v-model.number="botSeatNumber"
                type="number"
                min="0"
                :max="table.max_seats - 1"
              />
            </label>
          </fieldset>
          <ol class="variant-list">
            <li
              v-for="[key, name] in personalityOptions"
              :key="key"
              :class="{ picked: botPersonalityKey === key }"
            >
              <label>
                <input
                  type="radio"
                  name="bot-personality"
                  :value="key"
                  v-model="botPersonalityKey"
                />
                <span class="vname">{{ name }}</span>
              </label>
            </li>
          </ol>
          <div class="modal-actions">
            <button :disabled="busy" @click="botPickerOpen = false">cancel</button>
            <button :disabled="busy || botSeatNumber < 0" @click="addBot">
              add
            </button>
          </div>
        </div>
      </div>

      <!-- Variant picker modal -->
      <div v-if="pickerOpen" class="modal-backdrop" @click.self="pickerOpen = false">
        <div class="modal" role="dialog" aria-label="Choose variant">
          <h3>Choose variant</h3>
          <p class="muted">{{ seats.length }} seated</p>
          <ol class="variant-list">
            <li
              v-for="v in variants"
              :key="v.id"
              :class="{ disabled: !variantFits(v), picked: pickedVariantKey === v.key }"
            >
              <label :title="variantTooltip(v)">
                <input
                  type="radio"
                  name="variant"
                  :value="v.key"
                  :disabled="!variantFits(v)"
                  v-model="pickedVariantKey"
                />
                <span class="vname">{{ v.name }}</span>
                <span class="vrule">{{ variantRuleSummary(v) }}</span>
                <span v-if="!variantFits(v)" class="vwarn">
                  needs ≤ {{ v.max_seats }} seats
                </span>
              </label>
            </li>
          </ol>
          <div class="modal-actions">
            <button :disabled="busy" @click="pickerOpen = false">cancel</button>
            <button
              :disabled="busy || !variants.find((v) => v.key === pickedVariantKey && variantFits(v))"
              @click="startHand"
            >
              deal
            </button>
          </div>
        </div>
      </div>

      <!-- Action panel -->
      <div v-if="isMyTurn" class="actions">
        <span class="your-turn">your turn</span>
        <button :disabled="busy" @click="submitAction('fold')">fold</button>
        <button v-if="toCall === 0" :disabled="busy" @click="submitAction('check')">check</button>
        <button v-else :disabled="busy" @click="submitAction('call')">
          call {{ toCall }}
        </button>
        <template v-if="currentBet === 0">
          <input
            v-model.number="betAmount"
            type="number"
            :min="table.big_blind"
            :placeholder="`bet ≥ ${table.big_blind}`"
          />
          <button
            :disabled="busy || betAmount < table.big_blind"
            @click="submitAction('bet', betAmount)"
          >
            bet {{ betAmount }}
          </button>
        </template>
        <template v-else>
          <input
            v-model.number="betAmount"
            type="number"
            :min="minRaiseTotal"
            :placeholder="`raise to ≥ ${minRaiseTotal}`"
          />
          <button
            :disabled="busy || betAmount < minRaiseTotal"
            @click="submitAction('raise', betAmount - myRoundCommit)"
          >
            raise to {{ betAmount }}
          </button>
        </template>
        <button :disabled="busy" @click="submitAction('all_in')">all-in</button>
      </div>

      <!-- Hand-complete banner: pause to review winner + ready up -->
      <div v-if="isHandComplete" class="complete-banner">
        <h3>Hand complete</h3>
        <ul class="winners">
          <li v-for="w in hand?.winner_seats ?? []" :key="w.seat">
            <strong>{{ seatLabel(w.seat) }}</strong> won {{ w.amount }} · {{ w.class }}
          </li>
        </ul>
        <p v-if="!everyoneReady" class="muted">
          Waiting on {{ notReadySeats.length }} player{{ notReadySeats.length === 1 ? '' : 's' }}
          to ready up…
        </p>
        <p v-else class="muted">All players ready.</p>
        <div v-if="isSeated" class="ready-controls">
          <button v-if="!myReady" :disabled="busy" class="primary" @click="setReady(true)">
            ready for next hand
          </button>
          <button v-else :disabled="busy" @click="setReady(false)">unready</button>
        </div>
      </div>

      <p v-if="error" class="err">{{ error }}</p>

      <!-- Action log (compact, debug-flavored for v1) -->
      <details v-if="hand?.actions?.length" class="log">
        <summary>action log ({{ hand.actions.length }})</summary>
        <ol>
          <li v-for="a in hand.actions" :key="a.sequence">
            #{{ a.sequence }} · {{ a.phase }} · seat {{ a.seat }} · {{ a.type }}<span v-if="a.amount"> {{ a.amount }}</span>
          </li>
        </ol>
      </details>
    </template>
  </section>
</template>

<style scoped>
/* Full-viewport column layout. The page is split into compact header /
   opponents ribbon / flex-grow board / persistent me-dock / controls /
   action panel, so the three things you need every second of play
   (your hole cards, the board, the action buttons) stay on screen
   without scrolling. The .board flex:1 absorbs slack space, keeping
   the cards big and centered regardless of seat count. */
.table-view {
  max-width: 50rem;
  margin: 0 auto;
  padding: 0.5rem;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
header {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 0.4rem 0.8rem;
  flex-shrink: 0;
}
header h2 {
  margin: 0;
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 1.15rem;
}
header .meta {
  font-size: 0.8rem;
  opacity: 0.75;
}
header .phase {
  margin-left: auto;
  font-family: monospace;
  opacity: 0.85;
  font-size: 0.85rem;
}

/* Opponents grid: wrapping, uniform-width tiles. auto-fit + minmax
   gives us as many columns as fit (6+ on a wide desktop, 2-3 on
   mobile) with all tiles the SAME width regardless of name length —
   long names truncate via ellipsis on .opp-name. No horizontal scroll. */
.opponents {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(8.5rem, 1fr));
  gap: 0.4rem;
  flex-shrink: 0;
}
.opponent {
  border: 1.5px solid #333;
  background: #1a1c24;
  border-radius: 5px;
  padding: 0.35rem 0.45rem;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.78rem;
  min-width: 0; /* required for ellipsis on .opp-name to actually clamp */
}
.opponent.empty {
  border-style: dashed;
  background: transparent;
  opacity: 0.55;
  font-style: italic;
  align-items: center;
  justify-content: center;
  min-height: 3.4rem;
}
.opponent.empty .empty-label {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.1rem;
}
.opponent.is-current {
  border-color: #ee9;
  box-shadow: 0 0 0 2px #ee9, 0 0 8px -2px rgba(238, 238, 153, 0.45);
}
.opponent.folded {
  opacity: 0.5;
}
.opp-head {
  display: flex;
  gap: 0.35rem;
  align-items: center;
  min-width: 0;
}
.opp-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  font-size: 0.8rem;
}
/* Inline remove button. flex: 0 0 auto reserves its own space, so the
   name truncates around it instead of getting overlapped. */
.opponent .remove-bot-btn {
  flex: 0 0 auto;
  padding: 0 0.35rem;
  font-size: 0.9rem;
  line-height: 1;
  border-radius: 3px;
}
.opp-meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
  min-width: 0;
}
.opp-stack {
  font-family: monospace;
  font-size: 0.85rem;
  color: #cc6;
  flex-shrink: 0;
}
.opp-badges {
  display: flex;
  gap: 0.15rem;
  flex-wrap: wrap;
}
.opp-cards {
  display: flex;
  flex-wrap: wrap;
  gap: 0.15rem;
  min-width: 0;
}
.opp-tag {
  font-size: 0.7rem;
  font-style: italic;
  opacity: 0.7;
}
.opp-winner {
  color: #4d4;
  font-weight: 600;
  font-size: 0.78rem;
}
.seat-num {
  opacity: 0.5;
  font-family: monospace;
  font-size: 0.7rem;
}

/* Cards. Three sizes share base styles: mini (opponent), default
   (legacy), big (board + me-dock hole cards). */
.card {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  padding: 0.15rem 0.4rem;
  border: 1px solid #555;
  border-radius: 3px;
  background: #f4f1ea;
  color: #111;
  letter-spacing: 0.02em;
}
.card.red { color: #c33; }
.card.black { color: #111; }
.card.hidden {
  background: #2a3a2a;
  color: #888;
  border-color: #3a4a3a;
  opacity: 0.85;
}
.card.mini {
  font-size: 0.7rem;
  padding: 0.05rem 0.25rem;
  letter-spacing: 0;
}
.card.big {
  font-size: 1.4rem;
  padding: 0.35rem 0.6rem;
  min-width: 1.6rem;
  text-align: center;
}
.card.chosen {
  border-color: #4c4;
  box-shadow: 0 0 0 2px #4c4;
}

/* Board: visual centerpiece. flex:1 absorbs leftover vertical space. */
.board {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  border: 1px solid #2a4a2a;
  border-radius: 8px;
  min-height: 5rem;
  background: linear-gradient(180deg, #14241a 0%, #0e1a13 100%);
}
.board-cards {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.5rem;
  align-items: center;
  min-height: 2.4rem;
}
.pot-display {
  font-size: 0.9rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  opacity: 0.85;
}
.pot-display strong {
  font-size: 1.1rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: #ee9;
  margin-left: 0.25rem;
}

/* Me-dock: large persistent tile right above the action panel. */
.me-dock {
  border: 2px solid #4af;
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  background: linear-gradient(180deg, rgba(70, 170, 255, 0.10) 0%, #1a1c24 70%);
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  flex-shrink: 0;
}
.me-dock.is-current {
  box-shadow: 0 0 0 2px #ee9, 0 0 12px -2px rgba(238, 238, 153, 0.5);
}
.me-dock.folded {
  opacity: 0.6;
}
.dock-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.dock-id {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.dock-name {
  font-weight: 700;
  font-size: 0.95rem;
}
.dock-badges {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
}
.dock-stack {
  margin-left: auto;
  font-size: 0.8rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  opacity: 0.85;
}
.dock-stack strong {
  font-size: 1.05rem;
  font-family: monospace;
  color: #ee9;
  margin-left: 0.25rem;
}
.dock-cards {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  justify-content: center;
}
.dock-winner {
  color: #4d4;
  font-weight: 600;
  text-align: center;
  font-size: 0.9rem;
}

/* Badges (shared between opponents and me-dock). */
.badge {
  display: inline-flex;
  align-items: center;
  padding: 0.05rem 0.35rem;
  background: #2a2c38;
  color: #ccc;
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  border-radius: 2px;
  line-height: 1.4;
}
.badge.dealer { background: #2e5d8a; color: #e6f0fa; }
.badge.dealer.next {
  background: transparent;
  color: #87b4dc;
  border: 1px dashed #4d7aa5;
  padding: 0 0.25rem;
}
.badge.sb { background: #8a6f2c; color: #fff5d6; }
.badge.bb { background: #a35420; color: #ffe6d0; }
.badge.actor { background: #ee9; color: #16171d; }
.badge.ready { background: #2a5a2a; color: #cfc; }

.muted { opacity: 0.5; }

/* Controls. Sit/Leave/Add bot/Start hand. */
.controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  flex-shrink: 0;
}
fieldset {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  border: 1px solid #333;
  padding: 0.4rem 0.6rem;
  margin: 0;
  min-width: 0;
}
fieldset input { width: 5rem; }
.dealer-wait {
  font-style: italic;
  font-size: 0.9rem;
}
.add-bot-btn {
  background: #2a3d5a;
  color: #cfe;
  border-color: #4a6c95;
}
.hint {
  font-size: 0.85rem;
  color: #cb8;
  font-style: italic;
}
.remove-bot-btn {
  background: transparent;
  color: #d99;
  border: 1px dashed #644;
  border-radius: 3px;
  cursor: pointer;
  font-size: 0.7rem;
  padding: 0.1rem 0.5rem;
}
.remove-bot-btn:hover:not(:disabled) {
  background: #2a1818;
  color: #fbb;
}
.bot-seat-pick {
  margin: 0.5rem 0;
  border: 1px solid #333;
  padding: 0.4rem 0.6rem;
}

/* Action panel. Sits in normal flow at the bottom of the column. */
.actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  padding: 0.6rem 0.75rem;
  border: 1px solid #ee9;
  border-radius: 6px;
  background: #1a1a0c;
  box-shadow: 0 0 0 2px rgba(238, 238, 153, 0.15);
  flex-shrink: 0;
}
.actions input { width: 6rem; }
.your-turn {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #ee9;
  font-weight: 600;
  padding-right: 0.25rem;
}

/* Hand-complete banner, error, log */
.complete-banner {
  padding: 0.6rem 0.85rem;
  border: 1px solid #4d4;
  border-radius: 6px;
  background: #0f1f0f;
  flex-shrink: 0;
}
.complete-banner h3 {
  margin: 0 0 0.4rem;
  color: #cfc;
  font-size: 1rem;
}
.winners {
  list-style: none;
  padding: 0;
  margin: 0 0 0.4rem;
  display: grid;
  gap: 0.2rem;
  font-size: 0.9rem;
}
.winners strong { color: #cfc; }
.ready-controls {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.4rem;
}
.ready-controls .primary {
  background: #2a5a2a;
  color: #efe;
  border-color: #4c4;
}
.err { color: #d44; }
.log {
  font-size: 0.8rem;
  font-family: monospace;
  flex-shrink: 0;
}
.log ol { margin: 0.25rem 0 0 1rem; padding: 0; }

.variant-tag {
  font-size: 0.85rem;
  padding: 0.15rem 0.5rem;
  border: 1px solid #466;
  border-radius: 999px;
  background: #122;
  cursor: help;
}
.qmark {
  display: inline-block;
  margin-left: 0.3rem;
  width: 1rem;
  height: 1rem;
  line-height: 1rem;
  text-align: center;
  border-radius: 50%;
  background: #344;
  font-size: 0.7rem;
  opacity: 0.8;
}

/* Modals (unchanged from prior layout). */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.modal {
  background: #1c1c1c;
  border: 1px solid #444;
  border-radius: 6px;
  padding: 1rem 1.25rem;
  min-width: 22rem;
  max-width: 32rem;
  width: 90%;
  max-height: 90vh;
  overflow-y: auto;
}
.modal h3 { margin: 0 0 0.25rem; }
.variant-list {
  list-style: none;
  padding: 0;
  margin: 0.5rem 0 1rem;
  display: grid;
  gap: 0.25rem;
}
.variant-list li {
  border: 1px solid #333;
  border-radius: 4px;
}
.variant-list li.disabled { opacity: 0.45; }
.variant-list li.picked { border-color: #ee9; }
.variant-list label {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 0.5rem;
  align-items: center;
  padding: 0.4rem 0.6rem;
  cursor: pointer;
}
.variant-list li.disabled label { cursor: not-allowed; }
.vname { font-weight: 600; }
.vrule {
  font-size: 0.75rem;
  opacity: 0.75;
  font-family: monospace;
}
.vwarn {
  font-size: 0.7rem;
  color: #d99;
  white-space: nowrap;
}
.modal-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
}

/* Mobile tweaks. The whole layout is already viewport-driven — these
   just shrink padding and font sizes so opponent tiles fit at least
   3-up before scrolling kicks in, and the action panel stays
   thumb-friendly. No more sticky positioning needed: the column flex
   plus min-height: 100dvh already pin actions to the bottom. */
@media (max-width: 640px) {
  .table-view {
    padding: 0.4rem;
    gap: 0.4rem;
  }
  header h2 { font-size: 1rem; }
  header .meta {
    width: 100%;
    order: 3;
    font-size: 0.75rem;
  }
  header .phase { font-size: 0.8rem; }

  .opponents {
    /* Slightly smaller minimum on mobile so a 5-handed table fits two
       cleanly per row at ~360px viewport. */
    grid-template-columns: repeat(auto-fit, minmax(7rem, 1fr));
    gap: 0.35rem;
  }
  .opponent {
    padding: 0.25rem 0.35rem;
    font-size: 0.72rem;
  }
  .opp-name { font-size: 0.75rem; }
  .opp-stack { font-size: 0.78rem; }

  .board {
    padding: 0.6rem;
    min-height: 4rem;
  }
  .card.big {
    font-size: 1.15rem;
    padding: 0.25rem 0.45rem;
    min-width: 1.4rem;
  }

  .me-dock {
    padding: 0.4rem 0.5rem;
    gap: 0.3rem;
  }
  .dock-name { font-size: 0.9rem; }
  .dock-stack { font-size: 0.75rem; }
  .dock-stack strong { font-size: 0.95rem; }

  .controls { gap: 0.4rem; }
  fieldset {
    width: 100%;
    flex-direction: column;
    align-items: stretch;
  }
  fieldset label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  fieldset input { width: 8rem; }
  fieldset button { width: 100%; }
  .controls > button {
    flex: 1 1 auto;
    min-width: 8rem;
  }
  .dealer-wait {
    flex-basis: 100%;
    text-align: center;
  }

  .actions {
    /* 2-up grid for thumb-size buttons. The input+button pair gets a
       full row by itself. */
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.4rem;
    padding: 0.5rem;
  }
  .actions .your-turn {
    grid-column: 1 / -1;
    text-align: center;
    padding: 0;
  }
  .actions button { width: 100%; }
  .actions input {
    grid-column: 1 / -1;
    width: 100%;
  }
  .actions input + button { grid-column: 1 / -1; }

  .modal-backdrop {
    align-items: flex-end;
    padding: 0;
  }
  .modal {
    width: 100%;
    min-width: 0;
    max-width: none;
    max-height: 92vh;
    border-radius: 12px 12px 0 0;
    padding: 0.85rem 0.85rem max(0.85rem, env(safe-area-inset-bottom));
  }
  .variant-list label {
    grid-template-columns: auto 1fr;
    gap: 0.4rem 0.6rem;
    padding: 0.55rem 0.6rem;
  }
  .vrule {
    grid-column: 1 / -1;
    padding-left: 1.6rem;
  }
  .vwarn {
    grid-column: 1 / -1;
    padding-left: 1.6rem;
    white-space: normal;
  }
}
</style>
