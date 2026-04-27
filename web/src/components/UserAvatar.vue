<script setup lang="ts">
import { computed } from 'vue'
import { useAuth } from '../composables/useAuth'

interface UserLike {
  id: string
  email?: string
  name?: string
  avatar?: string
}

const props = withDefaults(
  defineProps<{
    user: UserLike | null | undefined
    /** CSS pixel size of the avatar circle. */
    size?: number
  }>(),
  { size: 32 },
)

const { avatarUrl } = useAuth()

// Request a thumbnail close to the rendered size (×2 for retina) so PB
// serves a smaller image. PB's thumb format is "WxH".
const url = computed(() => {
  if (!props.user) return null
  const px = Math.max(32, props.size * 2)
  return avatarUrl(props.user, `${px}x${px}`)
})

// Initials fallback: first two letters of name (or email prefix), upper.
const initials = computed(() => {
  if (!props.user) return ''
  const source = props.user.name?.trim() || props.user.email?.split('@')[0] || ''
  if (!source) return '?'
  const parts = source.split(/[\s._-]+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return source.slice(0, 2).toUpperCase()
})

// Stable per-id hue so each user gets a consistent fallback tint.
const hue = computed(() => {
  if (!props.user) return 220
  let h = 0
  for (const ch of props.user.id) h = (h * 31 + ch.charCodeAt(0)) % 360
  return h
})
</script>

<template>
  <span
    class="avatar"
    :style="{
      width: size + 'px',
      height: size + 'px',
      fontSize: Math.round(size * 0.4) + 'px',
      background: url ? '#222' : `hsl(${hue} 45% 35%)`,
    }"
    :title="user?.name || user?.email || ''"
  >
    <img v-if="url" :src="url" alt="" />
    <span v-else class="ini">{{ initials }}</span>
  </span>
</template>

<style scoped>
.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  overflow: hidden;
  color: #fff;
  font-weight: 600;
  letter-spacing: 0.02em;
  flex-shrink: 0;
  user-select: none;
  line-height: 1;
}
.avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.ini {
  text-shadow: 0 1px 1px rgba(0, 0, 0, 0.4);
}
</style>
