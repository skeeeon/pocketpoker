import { computed, ref } from 'vue'
import { pb } from '../pb'

interface AuthRecord {
  id: string
  email: string
  name?: string
  avatar?: string
}

const currentUser = ref<AuthRecord | null>(seed())

function seed(): AuthRecord | null {
  if (pb.authStore.isValid && pb.authStore.record) {
    const r = pb.authStore.record as unknown as AuthRecord
    return { id: r.id, email: r.email, name: r.name, avatar: r.avatar }
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

  async function register(email: string, password: string, name?: string) {
    // emailVisibility=true is also set by the server's OnRecordCreate
    // hook for this collection, but pass it from the client too so the
    // intent is obvious from the SPA side and so a record fetched
    // immediately after create has the flag set without a re-read.
    await pb.collection('users').create({
      email,
      password,
      passwordConfirm: password,
      name: name?.trim() || undefined,
      emailVisibility: true,
    })
    await login(email, password)
  }

  function logout() {
    pb.authStore.clear()
  }

  // updateProfile patches the current user's name and/or avatar.
  // Pass null for `avatar` to clear the existing image. Form data is
  // used so the avatar (a File) is uploaded as multipart instead of
  // being JSON-stringified into oblivion.
  async function updateProfile(args: { name?: string; avatar?: File | null }) {
    if (!currentUser.value) throw new Error('not signed in')
    const fd = new FormData()
    if (args.name !== undefined) fd.append('name', args.name.trim())
    if (args.avatar instanceof File) fd.append('avatar', args.avatar)
    else if (args.avatar === null) fd.append('avatar', '')
    const updated = await pb.collection('users').update(currentUser.value.id, fd)
    // PB's authStore record holds a stale copy until we save it back.
    pb.authStore.save(pb.authStore.token, updated)
  }

  // avatarUrl resolves the server URL for a user's stored avatar file,
  // returning null when the user has no avatar set. Pass a thumb spec
  // (e.g. "64x64") to use PB's on-the-fly thumbnail endpoint.
  function avatarUrl(
    u: { id: string; avatar?: string } | null | undefined,
    thumb?: string,
  ): string | null {
    if (!u || !u.avatar) return null
    const opts = thumb ? { thumb } : undefined
    return pb.files.getURL({ id: u.id, collectionName: 'users' }, u.avatar, opts)
  }

  return {
    user: computed(() => currentUser.value),
    isAuthed: computed(() => currentUser.value !== null),
    login,
    register,
    logout,
    updateProfile,
    avatarUrl,
  }
}
