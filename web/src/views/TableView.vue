<script setup lang="ts">
import { computed, ref, toRef, watch, watchEffect } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '../pb'
import { useAuth } from '../composables/useAuth'
import { useTable } from '../composables/useTable'
import { usePlayerHand } from '../composables/usePlayerHand'
import { useVariants, variantRuleSummary, type Variant } from '../composables/useVariants'

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

// Compute the seat that will deal the NEXT hand, mirroring server logic.
// First hand at the table (no `hand` record ever loaded): lowest active
// seat. Otherwise: next active clockwise from the previous dealer.
const nextDealerSeat = computed<number | null>(() => {
  const active = seats.value.filter((s) => s.status === 'active').sort((a, b) => a.seat_number - b.seat_number)
  if (active.length === 0) return null
  if (!hand.value) {
    return active[0].seat_number
  }
  const prev = table.value?.current_dealer_seat ?? -1
  const idx = active.findIndex((s) => s.seat_number === prev)
  if (idx < 0) return active[0].seat_number
  return active[(idx + 1) % active.length].seat_number
})
const isDealerMe = computed(() => {
  if (!mySeat.value || nextDealerSeat.value === null) return false
  return mySeat.value.seat_number === nextDealerSeat.value
})

// Sitting down
const sitSeatNumber = ref<number>(0)
const sitBuyIn = ref<number>(0)
function nextOpenSeat(): number {
  if (!table.value) return 0
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

function seatLabel(seatNum: number): string {
  const u = userBySeat.value[seatNum]
  if (!u) return `seat ${seatNum}`
  const me = user.value?.id === u.id
  const name = u.name?.trim() || u.email || u.id.slice(0, 6)
  return `${name}${me ? ' (you)' : ''}`
}

// init defaults when table loads
watch(table, (t) => {
  if (t && sitBuyIn.value === 0) sitBuyIn.value = t.buy_in
  if (sitSeatNumber.value === 0) {
    const next = nextOpenSeat()
    if (next >= 0) sitSeatNumber.value = next
  }
})
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

      <!-- Seats grid -->
      <ol class="seats">
        <li
          v-for="i in table.max_seats"
          :key="i"
          class="seat"
          :class="{
            empty: !seats.some((s) => s.seat_number === i - 1),
            'is-current': isCurrentActor(i - 1),
            'is-dealer': isDealer(i - 1),
          }"
        >
          <template v-if="seats.find((s) => s.seat_number === i - 1)">
            <div class="name">
              <span class="seat-num">#{{ i - 1 }}</span>
              {{ seatLabel(i - 1) }}
              <span v-if="isDealer(i - 1)" class="badge">D</span>
              <span v-if="hand && hand.small_blind_seat === i - 1" class="badge">SB</span>
              <span v-if="hand && hand.big_blind_seat === i - 1" class="badge">BB</span>
              <span
                v-if="isHandComplete && seats.find((s) => s.seat_number === i - 1)?.ready_for_next"
                class="badge ready"
                title="ready for next hand"
              >✓</span>
            </div>
            <div class="stack">stack {{ seats.find((s) => s.seat_number === i - 1)!.stack }}</div>
            <div class="status" v-if="statusForSeat(i - 1)">{{ statusForSeat(i - 1) }}</div>
            <div class="cards" v-if="hand">
              <template v-if="findHoleCardsForSeat(i - 1).length">
                <span
                  v-for="c in findHoleCardsForSeat(i - 1)"
                  :key="c"
                  class="card"
                  :class="[suitClass(c), { chosen: isCardChosenForSeat(i - 1, c) }]"
                >
                  {{ formatCard(c) }}
                </span>
              </template>
              <template v-else-if="seats.find((s) => s.seat_number === i - 1)">
                <span class="card hidden" v-for="n in holeCardCount" :key="n">??</span>
              </template>
            </div>
            <div v-if="winnerInfo(i - 1)" class="winner">
              won {{ winnerInfo(i - 1)!.amount }} · {{ winnerInfo(i - 1)!.class }}
            </div>
          </template>
          <template v-else>
            <div class="empty-label">empty seat {{ i - 1 }}</div>
          </template>
        </li>
      </ol>

      <!-- Community cards + pot -->
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
        </template>
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
.table-view {
  max-width: 50rem;
  margin: 0 auto;
}
header {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 0.5rem 1rem;
  margin-bottom: 0.75rem;
}
header h2 {
  margin: 0;
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
header .phase {
  margin-left: auto;
  font-family: monospace;
  opacity: 0.85;
}
.seats {
  list-style: none;
  padding: 0;
  margin: 0 0 1rem 0;
  display: grid;
  /* auto-fill keeps tiles ~10rem wide on tablets+ but collapses to
     2 cols on phones and 1 col on tiny screens. */
  grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr));
  gap: 0.5rem;
}
.seat {
  border: 1px solid #333;
  padding: 0.5rem;
  border-radius: 4px;
  font-size: 0.9rem;
  min-width: 0;
}
.seat .name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  header {
    margin-bottom: 0.5rem;
  }
  header h2 {
    font-size: 1.1rem;
  }
  header .meta {
    font-size: 0.8rem;
    opacity: 0.75;
    width: 100%;
    order: 3;
  }
  header .phase {
    font-size: 0.85rem;
  }
  .seats {
    grid-template-columns: repeat(2, 1fr);
    gap: 0.4rem;
  }
  .seat {
    padding: 0.4rem;
    font-size: 0.85rem;
  }
}
.seat.empty {
  opacity: 0.4;
  font-style: italic;
}
.seat.is-current {
  border-color: #ee9;
  box-shadow: 0 0 0 2px #ee9 inset;
}
.seat.is-dealer .badge:first-of-type {
  background: #466;
}
.badge {
  display: inline-block;
  padding: 0 0.3rem;
  margin-left: 0.25rem;
  background: #333;
  font-size: 0.7rem;
  border-radius: 2px;
}
.badge.ready {
  background: #2a5a2a;
  color: #cfc;
}
.seat-num {
  opacity: 0.55;
  font-family: monospace;
  font-size: 0.8rem;
  margin-right: 0.3rem;
}
.stack {
  font-family: monospace;
}
.status {
  font-size: 0.75rem;
  font-style: italic;
  opacity: 0.8;
}
.cards {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-top: 0.25rem;
}
.card {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  padding: 0.15rem 0.4rem;
  border: 1px solid #555;
  border-radius: 3px;
  background: #f4f1ea;
  color: #111;
  letter-spacing: 0.02em;
}
.card.red {
  color: #c33;
}
.card.black {
  color: #111;
}
.card.hidden {
  background: #2a3a2a;
  color: #888;
  border-color: #3a4a3a;
  opacity: 0.85;
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
.winner {
  color: #4d4;
  font-weight: 600;
  margin-top: 0.25rem;
}
.board {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: center;
  padding: 1rem;
  border: 1px solid #2a4a2a;
  border-radius: 8px;
  min-height: 2.5rem;
  margin-bottom: 0.75rem;
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

@media (max-width: 640px) {
  .board {
    padding: 0.75rem;
  }
  .card.big {
    font-size: 1.15rem;
    padding: 0.25rem 0.45rem;
    min-width: 1.4rem;
  }
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
.muted {
  opacity: 0.5;
}
.controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 0.75rem;
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
fieldset input {
  width: 5rem;
}

@media (max-width: 640px) {
  .controls {
    gap: 0.4rem;
  }
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
  fieldset input {
    width: 8rem;
  }
  fieldset button {
    width: 100%;
  }
  .controls > button {
    flex: 1 1 auto;
    min-width: 8rem;
  }
  .dealer-wait {
    flex-basis: 100%;
    text-align: center;
  }
}
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
}
.actions input {
  width: 6rem;
}
.your-turn {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #ee9;
  font-weight: 600;
  padding-right: 0.25rem;
}

@media (max-width: 640px) {
  .actions {
    /* Single grid that wraps cleanly: 2 buttons per row, input+button
       pair gets a full row by itself for thumb-size tap targets. */
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.4rem;
    padding: 0.5rem;
    /* Stay above the action bar even when scrolling the seat list. */
    position: sticky;
    bottom: 0.5rem;
  }
  .actions .your-turn {
    grid-column: 1 / -1;
    text-align: center;
    padding: 0;
  }
  .actions button {
    width: 100%;
  }
  .actions input {
    grid-column: 1 / -1;
    width: 100%;
  }
  /* The bet/raise button right after the input gets the full row too. */
  .actions input + button {
    grid-column: 1 / -1;
  }
}
.dealer-wait {
  font-style: italic;
  font-size: 0.9rem;
}
.complete-banner {
  margin-top: 0.75rem;
  padding: 0.75rem 1rem;
  border: 1px solid #4d4;
  border-radius: 6px;
  background: #0f1f0f;
}
.complete-banner h3 {
  margin: 0 0 0.5rem;
  color: #cfc;
}
.winners {
  list-style: none;
  padding: 0;
  margin: 0 0 0.5rem;
  display: grid;
  gap: 0.25rem;
}
.winners strong {
  color: #cfc;
}
.ready-controls {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
.ready-controls .primary {
  background: #2a5a2a;
  color: #efe;
  border-color: #4c4;
}
.err {
  color: #d44;
}
.log {
  margin-top: 1rem;
  font-size: 0.8rem;
  font-family: monospace;
}
.log ol {
  margin: 0.25rem 0 0 1rem;
  padding: 0;
}
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

@media (max-width: 640px) {
  .modal-backdrop {
    /* Bottom-sheet style: easier thumb reach. */
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
    /* Stack on phones so rule summary doesn't crowd the variant name. */
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
.modal h3 {
  margin: 0 0 0.25rem;
}
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
.variant-list li.disabled {
  opacity: 0.45;
}
.variant-list li.picked {
  border-color: #ee9;
}
.variant-list label {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 0.5rem;
  align-items: center;
  padding: 0.4rem 0.6rem;
  cursor: pointer;
}
.variant-list li.disabled label {
  cursor: not-allowed;
}
.vname {
  font-weight: 600;
}
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
</style>
