import { createRouter, createWebHistory } from 'vue-router'
import { pb } from './pb'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/lobby' },
    {
      path: '/login',
      name: 'login',
      component: () => import('./views/LoginView.vue'),
    },
    {
      path: '/lobby',
      name: 'lobby',
      component: () => import('./views/LobbyView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/table/:id',
      name: 'table',
      component: () => import('./views/TableView.vue'),
      meta: { requiresAuth: true },
      props: true,
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.requiresAuth && !pb.authStore.isValid) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && pb.authStore.isValid) {
    return { name: 'lobby' }
  }
})

// Reconnect-on-refresh: if the user is logged in and has an active seat
// at a table, jump straight there from the lobby on first navigation.
router.beforeEach(async (to, from) => {
  if (to.name !== 'lobby' || !pb.authStore.isValid || from.name) return
  try {
    const seats = await pb.collection('seats').getFullList({
      filter: `user = "${pb.authStore.record?.id}" && status = "active"`,
      perPage: 1,
    })
    if (seats.length > 0) {
      return { name: 'table', params: { id: seats[0].table } }
    }
  } catch {
    /* ignore — fall through to lobby */
  }
})

export default router
