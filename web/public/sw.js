// Minimal no-op service worker. Exists only so Chromium considers the
// app "installable" — we deliberately do not cache anything. All fetches
// pass through to the network. Update logic is also a no-op: skipWaiting
// + clients.claim so a refresh always picks up the latest assets.
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))
self.addEventListener('fetch', () => {})
