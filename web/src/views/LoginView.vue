<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuth } from '../composables/useAuth'

const { login, register } = useAuth()
const router = useRouter()

const email = ref('')
const password = ref('')
const mode = ref<'login' | 'register'>('login')
const error = ref<string | null>(null)
const busy = ref(false)

async function submit() {
  error.value = null
  busy.value = true
  try {
    if (mode.value === 'login') {
      await login(email.value, password.value)
    } else {
      await register(email.value, password.value)
    }
    router.push({ name: 'lobby' })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="login">
    <h2>{{ mode === 'login' ? 'Log in' : 'Create account' }}</h2>
    <form @submit.prevent="submit">
      <label>
        Email
        <input v-model="email" type="email" required autocomplete="email" />
      </label>
      <label>
        Password
        <input v-model="password" type="password" required minlength="8" autocomplete="current-password" />
      </label>
      <button type="submit" :disabled="busy">
        {{ busy ? '…' : mode === 'login' ? 'Log in' : 'Create' }}
      </button>
    </form>
    <p class="toggle">
      <a href="#" @click.prevent="mode = mode === 'login' ? 'register' : 'login'">
        {{ mode === 'login' ? 'Need an account?' : 'Have an account? Log in' }}
      </a>
    </p>
    <p v-if="error" class="err">{{ error }}</p>
  </section>
</template>

<style scoped>
.login {
  max-width: 24rem;
  margin: 4rem auto;
}
form {
  display: grid;
  gap: 0.75rem;
}
label {
  display: grid;
  gap: 0.25rem;
}
input {
  padding: 0.4rem 0.6rem;
}
.toggle {
  margin-top: 1rem;
  font-size: 0.9rem;
}
.err {
  color: #d33;
  margin-top: 1rem;
}

@media (max-width: 640px) {
  .login {
    margin: 1.5rem auto;
  }
  form button {
    width: 100%;
  }
}
</style>
