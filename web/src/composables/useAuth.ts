import { computed, ref } from 'vue'
import { pb } from '../pb'

interface AuthRecord {
  id: string
  email: string
  name?: string
}

const currentUser = ref<AuthRecord | null>(seed())

function seed(): AuthRecord | null {
  if (pb.authStore.isValid && pb.authStore.record) {
    const r = pb.authStore.record as unknown as AuthRecord
    return { id: r.id, email: r.email, name: r.name }
  }
  return null
}

pb.authStore.onChange(() => {
  currentUser.value = seed()
})

export function useAuth() {
  async function login(email: string, password: string) {
    await pb.collection('users').authWithPassword(email, password)
  }

  async function register(email: string, password: string) {
    await pb.collection('users').create({
      email,
      password,
      passwordConfirm: password,
    })
    await login(email, password)
  }

  function logout() {
    pb.authStore.clear()
  }

  return {
    user: computed(() => currentUser.value),
    isAuthed: computed(() => currentUser.value !== null),
    login,
    register,
    logout,
  }
}
