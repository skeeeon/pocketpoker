import { onMounted, ref } from 'vue'
import { pb } from '../pb'

export interface Variant {
  id: string
  key: string
  name: string
  hand_size: number
  min_from_hand: number
  max_from_hand: number
  min_from_board: number
  max_from_board: number
  max_seats: number
}

const variants = ref<Variant[]>([])
const byId = ref<Record<string, Variant>>({})
const byKey = ref<Record<string, Variant>>({})
const loaded = ref(false)
let inflight: Promise<void> | null = null

async function loadOnce() {
  if (loaded.value) return
  if (inflight) return inflight
  inflight = (async () => {
    try {
      const recs = await pb.collection('variants').getFullList({ sort: 'created' })
      const list = recs as unknown as Variant[]
      variants.value = list
      const id: Record<string, Variant> = {}
      const key: Record<string, Variant> = {}
      for (const v of list) {
        id[v.id] = v
        key[v.key] = v
      }
      byId.value = id
      byKey.value = key
      loaded.value = true
    } finally {
      inflight = null
    }
  })()
  return inflight
}

// Human-readable summary of how the chosen 5 cards are split between
// hole and board for this variant. Shown in the rules tooltip.
export function variantRuleSummary(v: Variant): string {
  const parts: string[] = []
  parts.push(`Hand: ${v.hand_size} hole card${v.hand_size === 1 ? '' : 's'}`)
  if (v.min_from_hand === v.max_from_hand) {
    parts.push(`use exactly ${v.min_from_hand} from hand`)
  } else {
    parts.push(`use ${v.min_from_hand}–${v.max_from_hand} from hand`)
  }
  if (v.min_from_board === v.max_from_board) {
    parts.push(`exactly ${v.min_from_board} from board`)
  } else {
    parts.push(`${v.min_from_board}–${v.max_from_board} from board`)
  }
  parts.push(`max ${v.max_seats} seats`)
  return parts.join(' · ')
}

export function useVariants() {
  onMounted(loadOnce)
  return { variants, byId, byKey, loaded, reload: loadOnce }
}
