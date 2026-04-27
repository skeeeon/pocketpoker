<script setup lang="ts">
import { useAuth } from './composables/useAuth'
import { useRouter } from 'vue-router'

const { user, logout } = useAuth()
const router = useRouter()

function onLogout() {
  logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <header>
    <h1>pocketpoker</h1>
    <nav v-if="user">
      <span class="who">{{ user.email }}</span>
      <button @click="onLogout">log out</button>
    </nav>
  </header>
  <main>
    <RouterView />
  </main>
</template>

<style scoped>
header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 0.5rem 1rem;
  border-bottom: 1px solid #333;
}
header h1 {
  margin: 0;
  font-size: 1.1rem;
}
nav {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}
nav .who {
  font-size: 0.85rem;
  opacity: 0.8;
}
main {
  padding: 1rem;
}
</style>
