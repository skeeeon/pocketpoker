import PocketBase from 'pocketbase'

// In dev (vite serve), the SPA runs on :5173 while PocketBase is on :8090,
// so we need an absolute cross-origin URL. In prod the SPA is served by
// the Go binary at the same origin as the API, so derive the base from
// window.location instead of baking 127.0.0.1 into the bundle — otherwise
// any user hitting the server from another host has their browser try to
// reach PocketBase on their *own* machine.
const url =
  import.meta.env.VITE_PB_URL ??
  (import.meta.env.DEV ? 'http://127.0.0.1:8090' : window.location.origin)

export const pb = new PocketBase(url)
