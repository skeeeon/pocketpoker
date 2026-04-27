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
export function usePlayerHand(handId: Ref<string>) {
  const myHand = ref<HandPlayer | null>(null)
  const allHands = ref<HandPlayer[]>([])

  let unsub: (() => void) | null = null

  async function load(id: string) {
    cleanup()
    if (!id || !pb.authStore.isValid) return
    try {
      const recs = await pb.collection('hand_players').getFullList({
        filter: `hand = "${id}"`,
      })
      allHands.value = recs as unknown as HandPlayer[]
      myHand.value =
        (allHands.value.find((h) => h.user === pb.authStore.record?.id) ?? null)
    } catch {
      myHand.value = null
      allHands.value = []
    }

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

  function cleanup() {
    if (unsub) {
      unsub()
      unsub = null
    }
  }

  watch(handId, (id) => load(id), { immediate: false })
  onMounted(() => load(handId.value))
  onUnmounted(cleanup)

  return { myHand, allHands }
}
