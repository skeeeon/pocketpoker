<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuth } from './composables/useAuth'
import UserAvatar from './components/UserAvatar.vue'
import ProfileModal from './components/ProfileModal.vue'

const { user, logout } = useAuth()
const router = useRouter()
const profileOpen = ref(false)

// Display label: name when set, otherwise the email's local part as a
// reasonable fallback (the bare email is too long for the chip on
// small screens).
const displayLabel = computed(() => {
  if (!user.value) return ''
  return user.value.name?.trim() || user.value.email.split('@')[0] || user.value.email
})

function onLogout() {
  logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <header>
    <h1>pocketpoker</h1>
    <nav v-if="user">
      <button class="chip" @click="profileOpen = true" :title="user.email">
        <UserAvatar :user="user" :size="28" />
        <span class="who">{{ displayLabel }}</span>
      </button>
      <button class="logout" @click="onLogout">log out</button>
    </nav>
  </header>
  <main>
    <RouterView />
  </main>

  <ProfileModal :open="profileOpen" @close="profileOpen = false" />
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
.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.2rem 0.6rem 0.2rem 0.25rem;
  background: #232531;
  border: 1px solid #2f3142;
  border-radius: 999px;
  color: inherit;
  cursor: pointer;
  min-width: 0;
}
.chip:hover {
  background: #2a2c3a;
}
.chip .who {
  font-size: 0.85rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 14ch;
}
.logout {
  font-size: 0.85rem;
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
  .chip .who {
    /* Hide the label on phones — the avatar alone identifies the user
       and the chip is still tappable to open the profile modal. */
    display: none;
  }
  .chip {
    padding: 0.2rem;
  }
  main {
    padding: 0.5rem;
  }
}
</style>
