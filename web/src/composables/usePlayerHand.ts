import { onMounted, onUnmounted, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { pb } from '../pb'

export interface HandPlayer {
  id: string
  hand: string
  seat: string
  user: string
  hole_cards: string[]
  status: 'active' | 'folded' | 'all_in'
}

// usePlayerHand subscribes to the caller's own hand_players row(s) for
// the given hand. Returns the row (single ref) plus, when the hand is
// complete and rules permit, the full list of opponents' hand_players.
//
// PocketBase realtime delivers events only when a record itself
// changes — it does NOT push records that *become* visible because of
// a rule re-evaluation triggered by a parent record change. So when
// hand.phase flips to "complete", opponents' hand_players rows stay
// invisible to the subscriber until the next explicit fetch. Callers
// must invoke `reload()` on that phase transition.
export function usePlayerHand(handId: Ref<string>) {
  const myHand = ref<HandPlayer | null>(null)
  const allHands = ref<HandPlayer[]>([])

  let unsub: (() => void) | null = null
  let currentId = ''

  async function fetchList(id: string) {
    if (!id || !pb.authStore.isValid) {
      myHand.value = null
      allHands.value = []
      return
    }
    try {
      const recs = await pb.collection('hand_players').getFullList({
        filter: `hand = "${id}"`,
      })
      allHands.value = recs as unknown as HandPlayer[]
      myHand.value =
        allHands.value.find((h) => h.user === pb.authStore.record?.id) ?? null
    } catch {
      myHand.value = null
      allHands.value = []
    }
  }

  async function load(id: string) {
    cleanup()
    currentId = id
    await fetchList(id)
    if (!id || !pb.authStore.isValid) return

    try {
      unsub = await pb.collection('hand_players').subscribe('*', (e) => {
        const rec = e.record as unknown as HandPlayer
        if (rec.hand !== id) return
        if (e.action === 'delete') {
          allHands.value = allHands.value.filter((h) => h.id !== rec.id)
        } else {
          const idx = allHands.value.findIndex((h) => h.id === rec.id)
          if (idx >= 0) allHands.value[idx] = rec
          else allHands.value = [...allHands.value, rec]
        }
        myHand.value =
          allHands.value.find((h) => h.user === pb.authStore.record?.id) ?? null
      })
    } catch {
      /* subscribe fails silently if rules block; that's fine */
    }
  }

  // Re-fetch hand_players for the currently subscribed hand without
  // tearing down the subscription. Used when `hand.phase` transitions
  // and previously-hidden opponent rows become readable.
  async function reload() {
    if (!currentId) return
    await fetchList(currentId)
  }

  function cleanup() {
    if (unsub) {
      unsub()
      unsub = null
    }
  }

  watch(handId, (id) => load(id), { immediate: false })
  onMounted(() => load(handId.value))
  onUnmounted(cleanup)

  return { myHand, allHands, reload }
}
