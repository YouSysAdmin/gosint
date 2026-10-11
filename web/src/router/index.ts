import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { storageGet, storageRemove, storageSet } from '@/composables/storage'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/auth/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/auth/RegisterView.vue'),
      meta: { public: true },
    },
    {
      path: '/invitations/:token',
      name: 'invitation',
      component: () => import('@/views/invitations/InvitationView.vue'),
      meta: { public: true },
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('@/views/auth/ForgotPasswordView.vue'),
      meta: { public: true },
    },
    {
      path: '/unseal',
      name: 'unseal',
      component: () => import('@/views/auth/UnsealView.vue'),
      meta: { public: true },
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      component: () => import('@/views/auth/ResetPasswordView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      children: [
        { path: '', redirect: '/investigations' },
        {
          path: 'investigations',
          name: 'investigations',
          component: () => import('@/views/investigations/InvestigationsView.vue'),
        },
        {
          path: 'investigations/:id',
          name: 'investigation',
          component: () => import('@/views/investigations/InvestigationView.vue'),
        },
        {
          path: 'investigations/:id/analyses/:aid',
          name: 'analysis',
          component: () => import('@/views/analyses/AnalysisView.vue'),
        },
        {
          path: 'sketches/:id',
          name: 'sketch',
          component: () => import('@/views/sketch/SketchView.vue'),
        },
        { path: 'profile', name: 'profile', component: () => import('@/views/ProfileView.vue') },
        { path: 'flows', name: 'flows', component: () => import('@/views/flows/FlowsView.vue') },
        {
          path: 'flows/new',
          name: 'flow-new',
          component: () => import('@/views/flows/FlowEditView.vue'),
        },
        {
          path: 'flows/:id',
          name: 'flow',
          component: () => import('@/views/flows/FlowEditView.vue'),
        },
        {
          path: 'transforms',
          name: 'transforms',
          component: () => import('@/views/transforms/TransformsView.vue'),
        },
        {
          path: 'templates/new',
          name: 'template-new',
          component: () => import('@/views/transforms/TemplateEditView.vue'),
        },
        {
          path: 'templates/:id',
          name: 'template',
          component: () => import('@/views/transforms/TemplateEditView.vue'),
        },
        {
          path: 'secrets',
          name: 'secrets',
          component: () => import('@/views/vault/VaultView.vue'),
        },
        {
          path: 'admin/users',
          name: 'admin-users',
          component: () => import('@/views/admin/UsersView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/providers',
          name: 'admin-providers',
          component: () => import('@/views/admin/ProvidersView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/types',
          name: 'admin-types',
          component: () => import('@/views/admin/TypesView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/models',
          name: 'admin-models',
          component: () => import('@/views/admin/ModelsView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/mail',
          name: 'admin-mail',
          component: () => import('@/views/admin/MailView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/worker',
          name: 'admin-worker',
          component: () => import('@/views/admin/WorkerView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/tools',
          name: 'admin-tools',
          component: () => import('@/views/admin/ToolsView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/files',
          name: 'admin-files',
          component: () => import('@/views/admin/FilesView.vue'),
          meta: { admin: true },
        },
        {
          path: 'admin/jobs',
          name: 'admin-jobs',
          component: () => import('@/views/admin/JobsView.vue'),
          meta: { admin: true },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFound.vue'),
      meta: { public: true },
    },
  ],
})

// Every page but the public ones needs a session, the admin pages an
// administrator.
router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const user = await useAuthStore().load()
  if (!user) return { name: 'login', query: { next: to.fullPath } }
  if (to.meta.admin && !user.admin) return { name: 'investigations' }
  return true
})

// A failed dynamic import means the tab holds a document older than the
// binary. Reload once, flagged so a genuinely missing chunk cannot loop.
const RELOAD_FLAG = 'gosint_chunk_reload'
router.onError((err) => {
  if (!/module script failed|dynamically imported module/i.test(String(err?.message))) return
  try {
    if (storageGet(RELOAD_FLAG, true)) return
    storageSet(RELOAD_FLAG, '1', true)
  } catch {
    return
  }
  window.location.reload()
})
router.afterEach(() => {
  try {
    storageRemove(RELOAD_FLAG, true)
  } catch {
    // storage unavailable, nothing to clear
  }
})

export default router
