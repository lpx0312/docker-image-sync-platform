import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '@/views/LoginView.vue'
import SyncView from '@/views/SyncView.vue'
import GitHubView from '@/views/GitHubView.vue'
import ConfigView from '@/views/ConfigView.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: LoginView,
    meta: { title: '登录', public: true }
  },
  {
    path: '/',
    redirect: '/sync'
  },
  {
    path: '/sync',
    name: 'Sync',
    component: SyncView,
    meta: { title: '镜像同步' }
  },
  {
    path: '/github',
    name: 'GitHub',
    component: GitHubView,
    meta: { title: 'GitHub Actions' }
  },
  {
    path: '/config',
    name: 'Config',
    component: ConfigView,
    meta: { title: '系统配置' }
  },
  {
    path: '/users',
    name: 'UserManage',
    component: () => import('@/views/UserManageView.vue'),
    meta: { title: '用户管理', requiresAdmin: true }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  if (to.meta.title) {
    document.title = `${to.meta.title} - Docker镜像同步平台`
  }

  const token = localStorage.getItem('token') || sessionStorage.getItem('token')

  if (to.meta.public) {
    if (token && to.name === 'Login') {
      next('/sync')
    } else {
      next()
    }
    return
  }

  if (!token) {
    next({ path: '/login', query: { redirect: to.fullPath } })
    return
  }

  if (to.meta.requiresAdmin) {
    const userStr = localStorage.getItem('user') || sessionStorage.getItem('user')
    const user = userStr ? JSON.parse(userStr) : null
    if (user?.role !== 'admin') {
      next('/sync')
      return
    }
  }

  next()
})

export default router
