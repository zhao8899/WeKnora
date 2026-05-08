import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const PLATFORM_HOME = '/platform/dashboard'
const TENANT_HOME = '/platform/home'

const getDefaultHomePath = (authStore: ReturnType<typeof useAuthStore>) =>
  authStore.canAccessAllTenants ? PLATFORM_HOME : TENANT_HOME

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'root',
      component: () => import('../views/auth/Login.vue'),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/auth/Login.vue'),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: '/join',
      name: 'joinOrganization',
      redirect: to => {
        const code = to.query.code as string
        return {
          path: '/platform/organizations',
          query: code ? { invite_code: code } : {}
        }
      },
      meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
    },
    {
      path: '/knowledgeBase',
      name: 'legacyHome',
      redirect: TENANT_HOME
    },
    {
      path: '/platform',
      name: 'Platform',
      redirect: TENANT_HOME,
      component: () => import('../views/platform/index.vue'),
      meta: { requiresInit: true, requiresAuth: true },
      children: [
        {
          path: 'dashboard',
          name: 'platformDashboard',
          component: () => import('../views/platform/PlatformHomeView.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'platform' }
        },
        {
          path: 'home',
          name: 'homeView',
          component: () => import('../views/home/HomeView.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'tenant',
          redirect: '/platform/settings'
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/settings/Settings.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'shared' }
        },
        {
          path: 'knowledge-bases',
          name: 'knowledgeBaseList',
          component: () => import('../views/knowledge/KnowledgeBaseList.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'faq',
          name: 'faqList',
          component: () => import('../views/knowledge/FAQList.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'knowledge-bases/:kbId',
          name: 'knowledgeBaseDetail',
          component: () => import('../views/knowledge/KnowledgeBase.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'knowledge-bases/:kbId/wiki',
          name: 'wikiWorkspace',
          component: () => import('../views/knowledge/wiki/WikiWorkspace.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'knowledge-bases/:kbId/wiki/graph',
          name: 'wikiGraph',
          component: () => import('../views/knowledge/wiki/WikiWorkspace.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'knowledge-search',
          name: 'knowledgeSearch',
          component: () => import('../views/knowledge/KnowledgeSearch.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'agents',
          name: 'agentList',
          component: () => import('../views/agent/AgentList.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'creatChat',
          name: 'globalCreatChat',
          component: () => import('../views/creatChat/creatChat.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'knowledge-bases/:kbId/creatChat',
          name: 'kbCreatChat',
          component: () => import('../views/creatChat/creatChat.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'chat/:chatid',
          name: 'chat',
          component: () => import('../views/chat/index.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'organizations',
          name: 'organizationList',
          component: () => import('../views/organization/OrganizationList.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'tenant' }
        },
        {
          path: 'usage-audit',
          name: 'usageAudit',
          component: () => import('../views/usage/UsageAudit.vue'),
          meta: { requiresInit: true, requiresAuth: true, roleScope: 'platform' }
        }
      ]
    }
  ]
})

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()
  const defaultHomePath = getDefaultHomePath(authStore)
  const isPlatformUser = authStore.canAccessAllTenants

  if (to.path === '/') {
    next(authStore.isLoggedIn ? defaultHomePath : '/login')
    return
  }

  if (to.meta.requiresAuth === false || to.meta.requiresInit === false) {
    if (to.path === '/login' && authStore.isLoggedIn) {
      next(defaultHomePath)
      return
    }
    next()
    return
  }

  if (to.meta.requiresAuth !== false && !authStore.isLoggedIn) {
    next('/login')
    return
  }

  const roleScope = to.meta.roleScope as 'platform' | 'tenant' | 'shared' | undefined
  if (roleScope === 'platform' && !isPlatformUser) {
    next(TENANT_HOME)
    return
  }

  if (roleScope === 'tenant' && isPlatformUser) {
    next(PLATFORM_HOME)
    return
  }

  if (to.path === '/platform' || to.path === '/platform/home' || to.path === '/platform/dashboard') {
    const expectedHome = isPlatformUser ? PLATFORM_HOME : TENANT_HOME
    if (to.path !== expectedHome) {
      next(expectedHome)
      return
    }
  }

  next()
})

export default router
