import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
      meta: { requiresAuth: false },
    },
    {
      path: '/callback',
      name: 'callback',
      component: () => import('../views/CallbackView.vue'),
      meta: { requiresAuth: false },
    },
    {
      path: '/',
      name: 'definitions',
      component: () => import('../views/DefinitionsListView.vue'),
      meta: { requiresAuth: true },
    },
  ],
})

router.beforeEach(async (to, _from, next) => {
  const authStore = useAuthStore()

  const requiresAuth = to.meta.requiresAuth ?? true

  if (requiresAuth) {
    sessionStorage.setItem('returnUrl', to.fullPath)
  }

  if (to.name !== 'callback' && to.name !== 'login') {
    await authStore.init()
  }

  if (requiresAuth && !authStore.isAuthenticated) {
    next({ name: 'login', query: { returnUrl: to.fullPath } })
  } else {
    next()
  }
})

export default router
