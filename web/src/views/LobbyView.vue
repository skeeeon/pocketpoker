<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '../pb'
import { useAuth } from '../composables/useAuth'

const router = useRouter()
const { user } = useAuth()

interface TableRow {
  id: string
  name: string
  buy_in: number
  small_blind: number
  big_blind: number
  status: string
}

const tables = ref<TableRow[]>([])
const error = ref<string | null>(null)

const showCreate = ref(false)
const newName = ref('')
const newBuyIn = ref(1000)
const newSb = ref(10)
const newBb = ref(20)
const creating = ref(false)

async function load() {
  try {
    const recs = await pb.collection('tables').getFullList({ sort: '-created' })
    tables.value = recs as unknown as TableRow[]
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)

async function createTable() {
  if (!user.value) return
  creating.value = true
  error.value = null
  try {
    const rec = await pb.collection('tables').create({
      name: newName.value || `Table ${Date.now()}`,
      created_by: user.value.id,
      buy_in: newBuyIn.value,
      small_blind: newSb.value,
      big_blind: newBb.value,
      max_seats: 8,
      status: 'waiting',
    })
    router.push({ name: 'table', params: { id: rec.id } })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    creating.value = false
  }
}

function joinTable(id: string) {
  router.push({ name: 'table', params: { id } })
}
</script>

<template>
  <section class="lobby">
    <header>
      <h2>Tables</h2>
      <button :class="{ primary: !showCreate }" @click="showCreate = !showCreate">
        {{ showCreate ? 'cancel' : '+ new table' }}
      </button>
    </header>

    <form v-if="showCreate" class="create" @submit.prevent="createTable">
      <label class="full">
        <span>Name</span>
        <input v-model="newName" type="text" placeholder="Friday night" />
      </label>
      <label>
        <span>Buy-in</span>
        <input v-model.number="newBuyIn" type="number" min="20" />
      </label>
      <label>
        <span>SB</span>
        <input v-model.number="newSb" type="number" min="1" />
      </label>
      <label>
        <span>BB</span>
        <input v-model.number="newBb" type="number" min="2" />
      </label>
      <button :disabled="creating" type="submit" class="primary">
        {{ creating ? '…' : 'create' }}
      </button>
    </form>

    <p v-if="error" class="err">{{ error }}</p>

    <ul v-if="tables.length" class="grid">
      <li v-for="t in tables" :key="t.id" class="card" @click="joinTable(t.id)">
        <div class="card-head">
          <span class="name">{{ t.name }}</span>
          <span class="status" :class="t.status">{{ t.status }}</span>
        </div>
        <div class="meta">
          <span class="pill mono">{{ t.small_blind }}/{{ t.big_blind }}</span>
          <span class="muted">buy-in {{ t.buy_in }}</span>
        </div>
        <span class="open">open →</span>
      </li>
    </ul>
    <p v-else class="empty muted">No tables yet — create one to start.</p>
  </section>
</template>

<style scoped>
.lobby {
  max-width: 42rem;
  margin: 0 auto;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.create {
  display: grid;
  grid-template-columns: 2fr 1fr 1fr 1fr auto;
  gap: 0.6rem;
  align-items: end;
  padding: 0.85rem;
  border: 1px solid var(--border);
  background: var(--bg-elev);
  border-radius: var(--radius);
  margin-bottom: 1rem;
}
.create label {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 0;
}
.create label.full { grid-column: 1 / -1; }
.create label > span {
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-muted);
}
.create input { width: 100%; }
.create > button { white-space: nowrap; }

.grid {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
  gap: 0.75rem;
}
.card {
  position: relative;
  background: var(--bg-elev);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 0.75rem 0.9rem;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  transition: border-color 120ms ease, transform 120ms ease, background 120ms ease;
}
.card:hover {
  border-color: var(--border-focus);
  background: var(--bg-elev-2);
  transform: translateY(-1px);
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.card .name {
  font-weight: 600;
  font-size: 1rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1 1 auto;
  min-width: 0;
}
.status {
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 0.1rem 0.4rem;
  border-radius: var(--radius-sm);
  background: var(--bg-input);
  color: var(--text-muted);
}
.status.active {
  background: rgba(102, 204, 102, 0.15);
  color: var(--good);
}
.status.waiting {
  background: rgba(238, 238, 153, 0.12);
  color: var(--warn);
}
.meta {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-size: 0.85rem;
}
.pill {
  display: inline-block;
  padding: 0.1rem 0.45rem;
  background: var(--accent-soft);
  color: var(--accent);
  border-radius: var(--radius-sm);
  font-weight: 600;
  font-size: 0.85rem;
}
.open {
  align-self: flex-end;
  font-size: 0.8rem;
  color: var(--accent);
  opacity: 0;
  transition: opacity 120ms ease;
}
.card:hover .open { opacity: 1; }

.empty {
  text-align: center;
  padding: 2rem 1rem;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  font-style: italic;
}

@media (max-width: 640px) {
  .create {
    grid-template-columns: repeat(3, 1fr);
  }
  .create label.full { grid-column: 1 / -1; }
  .create > button { grid-column: 1 / -1; width: 100%; }
  .grid { grid-template-columns: 1fr; }
  /* On phones the open hint doesn't trigger via :hover; show it. */
  .open { opacity: 0.8; }
}
</style>
