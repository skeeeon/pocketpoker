import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { pb } from '../pb'

export interface UserBrief {
  id: string
  email: string
  name?: string
  avatar?: string
}

export interface Seat {
  id: string
  table: string
  user: string
  seat_number: number
  stack: number
  status: 'active' | 'sitting_out' | 'disconnected'
  ready_for_next: boolean
  bot_personality?: string
}

// Personality display names. Mirrored from engine/bot.go Personalities;
// the wire value is the lowercase key, the UI shows the friendly name.
export const BOT_PERSONALITIES: Record<string, string> = {
  tight: 'Tight Tina',
  loose: 'Loose Larry',
  maniac: 'Maniac Mike',
  station: 'Calling Station Carl',
}

export interface HandAction {
  sequence: number
  seat: number
  phase: string
  type: string
  amount: number
}

export interface SeatResult {
  seat: number
  // null when the hand was won uncontested (everyone folded) — the
  // engine doesn't reveal cards in that case.
  cards: string[] | null
  rank: number
  class: string
  amount: number
}

export interface Hand {
  id: string
  table: string
  variant: string
  dealer_seat: number
  small_blind_seat: number
  big_blind_seat: number
  community_cards: string[] | null
  pot: number
  phase: 'preflop' | 'flop' | 'turn' | 'river' | 'showdown' | 'complete'
  current_actor_seat: number
  current_bet: number
  actions: HandAction[] | null
  version: number
  winner_seats: SeatResult[] | null
}

export interface Table {
  id: string
  name: string
  created_by: string
  buy_in: number
  small_blind: number
  big_blind: number
  max_seats: number
  current_dealer_seat: number
  current_hand: string
  status: 'waiting' | 'active'
}

// useTable subscribes to the table record, its seats, and (when present)
// the active hand. Returns reactive refs that update via PB realtime SSE.
export function useTable(tableId: Ref<string>) {
  const table = ref<Table | null>(null)
  const seats = ref<Seat[]>([])
  const hand = ref<Hand | null>(null)
  const users = ref<Record<string, UserBrief>>({})
  const error = ref<string | null>(null)

  const unsubFns: Array<() => void> = []

  async function loadUsersForSeats(seatRecs: Seat[]) {
    // Bot seats carry user="" and have no users record to fetch — skip
    // them or PB will 404 the OR filter.
    const ids = Array.from(
      new Set(seatRecs.map((s) => s.user).filter((id) => id !== '')),
    )
    const missing = ids.filter((id) => !users.value[id])
    if (!missing.length) return
    try {
      const filter = missing.map((id) => `id = "${id}"`).join(' || ')
      const recs = await pb.collection('users').getFullList({ filter })
      const next: Record<string, UserBrief> = { ...users.value }
      for (const r of recs as unknown as UserBrief[]) {
        next[r.id] = { id: r.id, email: r.email, name: r.name, avatar: r.avatar }
      }
      users.value = next
    } catch {
      /* permission may block; we fall back to id prefix in the UI */
    }
  }

  // Build the seat → display profile map. Humans pull from the users
  // cache; bots are synthesized from their personality so the existing
  // avatar/name rendering doesn't need bot-specific branches.
  const userBySeat = computed<Record<number, UserBrief | null>>(() => {
    const out: Record<number, UserBrief | null> = {}
    for (const s of seats.value) {
      if (s.bot_personality) {
        const display = BOT_PERSONALITIES[s.bot_personality] ?? s.bot_personality
        out[s.seat_number] = {
          id: `bot:${s.id}`,
          email: '',
          name: display,
        }
        continue
      }
      out[s.seat_number] = users.value[s.user] ?? null
    }
    return out
  })

  async function loadAndSubscribe(id: string) {
    cleanup()
    if (!id) return

    try {
      table.value = (await pb.collection('tables').getOne(id)) as unknown as Table
    } catch (e) {
      error.value = `failed to load table: ${(e as Error).message}`
      return
    }

    try {
      const seatRecs = await pb.collection('seats').getFullList({
        filter: `table = "${id}"`,
        sort: 'seat_number',
      })
      seats.value = seatRecs as unknown as Seat[]
      await loadUsersForSeats(seats.value)
    } catch (e) {
      error.value = `failed to load seats: ${(e as Error).message}`
    }

    if (table.value?.current_hand) {
      try {
        hand.value = (await pb.collection('hands').getOne(
          table.value.current_hand,
        )) as unknown as Hand
      } catch (e) {
        // hand may have been cleared between table load and now; ignore.
      }
    }

    unsubFns.push(
      await pb.collection('tables').subscribe(id, (e) => {
        table.value = e.record as unknown as Table
        if (table.value?.current_hand && (!hand.value || hand.value.id !== table.value.current_hand)) {
          pb.collection('hands')
            .getOne(table.value.current_hand)
            .then((rec) => {
              hand.value = rec as unknown as Hand
            })
            .catch(() => {})
        } else if (!table.value?.current_hand) {
          hand.value = null
        }
      }),
    )

    unsubFns.push(
      await pb.collection('seats').subscribe(
        '*',
        (e) => {
          const rec = e.record as unknown as Seat
          if (rec.table !== id) return
          if (e.action === 'delete') {
            seats.value = seats.value.filter((s) => s.id !== rec.id)
            return
          }
          const idx = seats.value.findIndex((s) => s.id === rec.id)
          if (idx >= 0) seats.value[idx] = rec
          else seats.value = [...seats.value, rec].sort((a, b) => a.seat_number - b.seat_number)
          loadUsersForSeats(seats.value)
        },
      ),
    )

    unsubFns.push(
      await pb.collection('hands').subscribe('*', (e) => {
        const rec = e.record as unknown as Hand
        if (rec.table !== id) return
        if (hand.value?.id === rec.id) hand.value = rec
        else if (table.value?.current_hand === rec.id) hand.value = rec
      }),
    )
  }

  function cleanup() {
    unsubFns.splice(0).forEach((fn) => {
      try {
        fn()
      } catch {
        /* noop */
      }
    })
  }

  watch(tableId, (id) => loadAndSubscribe(id), { immediate: false })
  onMounted(() => loadAndSubscribe(tableId.value))
  onUnmounted(cleanup)

  return { table, seats, hand, userBySeat, error }
}
