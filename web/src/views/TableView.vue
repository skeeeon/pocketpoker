<script setup lang="ts">
import { computed, ref, toRef } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '../pb'
import { useAuth } from '../composables/useAuth'
import { useTable } from '../composables/useTable'
import { usePlayerHand } from '../composables/usePlayerHand'

const props = defineProps<{ id: string }>()
const router = useRouter()
const { user } = useAuth()
const tableId = toRef(props, 'id')
const { table, seats, hand, userBySeat, error: tableError } = useTable(tableId)
const handId = computed(() => hand.value?.id ?? '')
const { myHand, allHands } = usePlayerHand(handId)

const error = ref<string | null>(null)
const busy = ref(false)

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

async function startHand() {
  if (!table.value) return
  busy.value = true
  error.value = null
  try {
    await pb.send(`/api/poker/tables/${table.value.id}/start-hand`, {
      method: 'POST',
      body: { variant_key: 'holdem' },
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

const betAmount = ref<number>(0)
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
function formatCard(c: string): string {
  return c // already in the form "As", "Td", etc.
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

function seatLabel(seatNum: number): string {
  const u = userBySeat.value[seatNum]
  if (!u) return `seat ${seatNum}`
  const me = user.value?.id === u.id
  const name = u.name?.trim() || u.email || u.id.slice(0, 6)
  return `${name}${me ? ' (you)' : ''}`
}

// init defaults when table loads
import { watch } from 'vue'
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
        <span v-if="hand" class="phase">{{ hand.phase }} · pot {{ hand.pot }}</span>
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
            </div>
            <div class="stack">stack {{ seats.find((s) => s.seat_number === i - 1)!.stack }}</div>
            <div class="status" v-if="statusForSeat(i - 1)">{{ statusForSeat(i - 1) }}</div>
            <div class="cards" v-if="hand">
              <template v-if="findHoleCardsForSeat(i - 1).length">
                <span v-for="c in findHoleCardsForSeat(i - 1)" :key="c" class="card">
                  {{ formatCard(c) }}
                </span>
              </template>
              <template v-else-if="seats.find((s) => s.seat_number === i - 1)">
                <span class="card hidden" v-for="n in 2" :key="n">??</span>
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

      <!-- Community cards -->
      <div class="board">
        <span v-if="!hand?.community_cards?.length" class="muted">— no board yet —</span>
        <span v-for="c in hand?.community_cards ?? []" :key="c" class="card big">
          {{ formatCard(c) }}
        </span>
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
            v-if="!hand || isHandComplete"
            :disabled="busy || seats.length < 3"
            @click="startHand"
          >
            start hand (Hold'em)
          </button>
        </template>
      </div>

      <!-- Action panel -->
      <div v-if="isMyTurn" class="actions">
        <button :disabled="busy" @click="submitAction('fold')">fold</button>
        <button v-if="toCall === 0" :disabled="busy" @click="submitAction('check')">check</button>
        <button v-else :disabled="busy" @click="submitAction('call')">
          call {{ toCall }}
        </button>
        <template v-if="toCall === 0">
          <input v-model.number="betAmount" type="number" :min="table.big_blind" :placeholder="`bet ≥ ${table.big_blind}`" />
          <button :disabled="busy || betAmount < table.big_blind" @click="submitAction('bet', betAmount)">
            bet
          </button>
        </template>
        <template v-else>
          <input v-model.number="betAmount" type="number" :min="hand?.current_bet ?? 0 + table.big_blind" :placeholder="`raise to ≥ ${(hand?.current_bet ?? 0) + table.big_blind}`" />
          <button
            :disabled="busy || betAmount < (hand?.current_bet ?? 0) + table.big_blind"
            @click="submitAction('raise', betAmount - myRoundCommit)"
          >
            raise to {{ betAmount }}
          </button>
        </template>
        <button :disabled="busy" @click="submitAction('all_in')">all-in</button>
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
  gap: 1rem;
  margin-bottom: 0.75rem;
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
  grid-template-columns: repeat(4, 1fr);
  gap: 0.5rem;
}
.seat {
  border: 1px solid #333;
  padding: 0.5rem;
  border-radius: 4px;
  font-size: 0.9rem;
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
  gap: 0.25rem;
  margin-top: 0.25rem;
}
.card {
  font-family: monospace;
  padding: 0.1rem 0.3rem;
  border: 1px solid #555;
  border-radius: 2px;
  background: #1a1a1a;
}
.card.hidden {
  opacity: 0.5;
}
.card.big {
  font-size: 1.2rem;
  padding: 0.25rem 0.5rem;
}
.winner {
  color: #4d4;
  font-weight: 600;
  margin-top: 0.25rem;
}
.board {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  padding: 0.75rem;
  border: 1px dashed #444;
  border-radius: 4px;
  min-height: 2.5rem;
  margin-bottom: 0.75rem;
}
.muted {
  opacity: 0.5;
}
.controls {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  margin-bottom: 0.75rem;
}
fieldset {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  border: 1px solid #333;
  padding: 0.4rem;
}
.actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
  padding: 0.5rem;
  border: 1px solid #ee9;
  border-radius: 4px;
}
.actions input {
  width: 6rem;
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
</style>
