import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'

createApp(App).use(router).mount('#app')

// Register the no-op service worker so Chromium offers "Install app".
// We deliberately don't cache anything — see public/sw.js.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      /* registration is best-effort; PWA install is a nice-to-have */
    })
  })
}
