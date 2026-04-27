<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAuth } from '../composables/useAuth'
import UserAvatar from './UserAvatar.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const { user, updateProfile } = useAuth()

const nameInput = ref('')
const avatarFile = ref<File | null>(null)
const previewUrl = ref<string | null>(null)
const clearAvatar = ref(false)
const busy = ref(false)
const error = ref<string | null>(null)

// When opening: seed the name input from the current user so partial
// edits don't surprise the user, and reset transient state from a
// previous session of the modal.
watch(
  () => props.open,
  (open) => {
    if (!open) return
    nameInput.value = user.value?.name ?? ''
    avatarFile.value = null
    clearAvatar.value = false
    error.value = null
    if (previewUrl.value) {
      URL.revokeObjectURL(previewUrl.value)
      previewUrl.value = null
    }
  },
  { immediate: true },
)

function onAvatarPicked(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] ?? null
  avatarFile.value = file
  clearAvatar.value = false
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = file ? URL.createObjectURL(file) : null
}

function onClearAvatar() {
  avatarFile.value = null
  clearAvatar.value = true
  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = null
  }
}

// Preview: file preview takes priority, then "cleared" placeholder,
// then existing avatar via UserAvatar's own fetch.
const previewUser = computed(() => {
  if (!user.value) return null
  if (clearAvatar.value) return { ...user.value, avatar: undefined }
  return user.value
})

async function save() {
  busy.value = true
  error.value = null
  try {
    await updateProfile({
      name: nameInput.value,
      avatar: clearAvatar.value ? null : avatarFile.value,
    })
    emit('close')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function close() {
  if (busy.value) return
  emit('close')
}
</script>

<template>
  <div v-if="open" class="backdrop" @click.self="close">
    <div class="modal" role="dialog" aria-label="Edit profile">
      <h3>Profile</h3>

      <div class="preview">
        <img v-if="previewUrl" :src="previewUrl" alt="avatar preview" class="preview-img" />
        <UserAvatar v-else :user="previewUser" :size="80" />
        <div class="who">
          <div class="email">{{ user?.email }}</div>
        </div>
      </div>

      <label class="row">
        <span>Display name</span>
        <input v-model="nameInput" type="text" maxlength="60" placeholder="(optional)" />
      </label>

      <div class="row">
        <span>Avatar</span>
        <div class="avatar-controls">
          <input type="file" accept="image/*" @change="onAvatarPicked" />
          <button
            v-if="user?.avatar && !clearAvatar && !avatarFile"
            type="button"
            class="link"
            @click="onClearAvatar"
          >
            remove current
          </button>
          <span v-if="clearAvatar" class="muted">avatar will be cleared on save</span>
        </div>
      </div>

      <p v-if="error" class="err">{{ error }}</p>

      <div class="actions">
        <button :disabled="busy" @click="close">cancel</button>
        <button :disabled="busy" class="primary" @click="save">save</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: 1rem;
}
.modal {
  background: #1c1d24;
  border: 1px solid #333;
  border-radius: 8px;
  padding: 1rem 1.25rem;
  max-width: 26rem;
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.modal h3 {
  margin: 0;
}
.preview {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.preview-img {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  object-fit: cover;
  background: #222;
}
.who {
  min-width: 0;
}
.email {
  font-size: 0.85rem;
  opacity: 0.75;
  word-break: break-all;
}
.row {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.row > span:first-child {
  font-size: 0.8rem;
  opacity: 0.75;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}
.row input[type='text'] {
  padding: 0.35rem 0.5rem;
  width: 100%;
}
.avatar-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
.link {
  background: none;
  border: none;
  color: #8af;
  cursor: pointer;
  padding: 0;
  text-decoration: underline;
  font-size: 0.85rem;
}
.muted {
  font-size: 0.8rem;
  opacity: 0.7;
  font-style: italic;
}
.err {
  color: #d66;
  margin: 0;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.25rem;
}
.actions .primary {
  background: #4af;
  color: #16171d;
  border: none;
  padding: 0.35rem 0.9rem;
  border-radius: 3px;
  font-weight: 600;
}
.actions .primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
