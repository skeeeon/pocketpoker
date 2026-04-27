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
      <button @click="showCreate = !showCreate">
        {{ showCreate ? 'cancel' : '+ new table' }}
      </button>
    </header>

    <form v-if="showCreate" class="create" @submit.prevent="createTable">
      <input v-model="newName" placeholder="table name" />
      <label>buy-in <input v-model.number="newBuyIn" type="number" min="20" /></label>
      <label>SB <input v-model.number="newSb" type="number" min="1" /></label>
      <label>BB <input v-model.number="newBb" type="number" min="2" /></label>
      <button :disabled="creating" type="submit">create</button>
    </form>

    <p v-if="error" class="err">{{ error }}</p>

    <ul v-if="tables.length" class="list">
      <li v-for="t in tables" :key="t.id" class="row">
        <span class="name">{{ t.name }}</span>
        <span class="meta">SB/BB {{ t.small_blind }}/{{ t.big_blind }} · buy-in {{ t.buy_in }} · {{ t.status }}</span>
        <button @click="joinTable(t.id)">open</button>
      </li>
    </ul>
    <p v-else class="empty">no tables yet — create one to start</p>
  </section>
</template>

<style scoped>
.lobby {
  max-width: 40rem;
  margin: 0 auto;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.5rem;
  margin-bottom: 1rem;
}
header h2 {
  margin: 0;
}
.create {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
  padding: 0.75rem;
  border: 1px dashed #444;
  margin-bottom: 1rem;
}
.create input {
  padding: 0.3rem 0.5rem;
}
.create > input,
.create label {
  flex: 1 1 auto;
  min-width: 0;
}
.create label input {
  width: 5rem;
  margin-left: 0.4rem;
}
.list {
  list-style: none;
  padding: 0;
  margin: 0;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #2a2a2a;
}
.row .name {
  font-weight: 600;
  flex: 1 1 auto;
}
.row .meta {
  font-size: 0.85rem;
  opacity: 0.75;
  flex: 1 1 100%;
}
.empty {
  opacity: 0.7;
  font-style: italic;
}
.err {
  color: #d33;
}

@media (max-width: 640px) {
  .create {
    flex-direction: column;
    align-items: stretch;
  }
  .create > input,
  .create > label,
  .create > button {
    width: 100%;
  }
  .create label {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .create label input {
    width: 8rem;
  }
  .row {
    padding: 0.6rem 0.5rem;
  }
  .row > button {
    width: 100%;
  }
}
</style>
