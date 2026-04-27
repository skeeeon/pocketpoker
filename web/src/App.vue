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
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  border-bottom: 1px solid #333;
  position: sticky;
  top: 0;
  background: #16171d;
  z-index: 30;
}
header h1 {
  margin: 0;
  font-size: 1.1rem;
}
nav {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  min-width: 0;
}
nav .who {
  font-size: 0.85rem;
  opacity: 0.8;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 18ch;
}
main {
  padding: 1rem;
}

@media (max-width: 640px) {
  header {
    padding: 0.4rem 0.6rem;
  }
  header h1 {
    font-size: 1rem;
  }
  nav .who {
    /* Email takes up too much room on phones; hide it. */
    display: none;
  }
  main {
    padding: 0.5rem;
  }
}
</style>
