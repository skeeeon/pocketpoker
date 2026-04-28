<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuth } from '../composables/useAuth'

const { login, register } = useAuth()
const router = useRouter()

const email = ref('')
const password = ref('')
const name = ref('')
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
      await register(email.value, password.value, name.value)
    }
    router.push({ name: 'lobby' })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function setMode(m: 'login' | 'register') {
  if (mode.value === m) return
  mode.value = m
  error.value = null
}
</script>

<template>
  <section class="login">
    <div class="card">
      <div class="brand">
        <span class="suit s">♠</span><span class="suit h">♥</span>
        <h1>pocketpoker</h1>
        <span class="suit d">♦</span><span class="suit c">♣</span>
      </div>

      <div class="mode-tabs" role="tablist">
        <button
          type="button"
          :class="{ active: mode === 'login' }"
          @click="setMode('login')"
        >Log in</button>
        <button
          type="button"
          :class="{ active: mode === 'register' }"
          @click="setMode('register')"
        >Create account</button>
      </div>

      <form class="form" @submit.prevent="submit">
        <label>
          <span>Email</span>
          <input v-model="email" type="email" required autocomplete="email" />
        </label>
        <label v-if="mode === 'register'">
          <span>Display name <em class="optional">optional</em></span>
          <input v-model="name" type="text" maxlength="60" autocomplete="nickname" />
        </label>
        <label>
          <span>Password</span>
          <input
            v-model="password"
            type="password"
            required
            minlength="8"
            :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
          />
        </label>
        <p v-if="error" class="err">{{ error }}</p>
        <button type="submit" class="primary submit" :disabled="busy">
          {{ busy ? '…' : mode === 'login' ? 'Log in' : 'Create account' }}
        </button>
      </form>

    </div>
  </section>
</template>

<style scoped>
.login {
  min-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem 1rem;
}
.card {
  width: 100%;
  max-width: 24rem;
  background: var(--bg-elev);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 1.75rem;
  box-shadow: var(--shadow);
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
}
.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}
.brand h1 {
  font-size: 1.4rem;
  letter-spacing: -0.01em;
}
.suit {
  font-size: 1rem;
  opacity: 0.7;
}
.suit.s, .suit.c { color: var(--text-muted); }
.suit.h, .suit.d { color: #d56; }

.mode-tabs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 0.2rem;
  gap: 0.2rem;
}
.mode-tabs button {
  background: transparent;
  border: none;
  color: var(--text-muted);
  padding: 0.4rem;
  font-size: 0.9rem;
  font-weight: 600;
  border-radius: calc(var(--radius) - 2px);
  min-height: auto;
}
.mode-tabs button:hover {
  background: rgba(255, 255, 255, 0.04);
  color: var(--text);
  border: none;
}
.mode-tabs button.active {
  background: var(--bg-elev-2);
  color: var(--text);
  box-shadow: var(--shadow-sm);
}

.form {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}
.form label {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.form label > span {
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-muted);
}
.optional {
  font-style: normal;
  font-weight: 400;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 0.25rem;
  color: var(--text-faint);
}

.submit {
  margin-top: 0.25rem;
}

@media (max-width: 640px) {
  .login {
    padding: 1rem 0.5rem;
    align-items: flex-start;
    padding-top: 1.5rem;
  }
  .card {
    padding: 1.25rem;
  }
  .mode-tabs button {
    min-height: 2.4rem;
  }
}
</style>
