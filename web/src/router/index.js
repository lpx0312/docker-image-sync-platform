import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '@/views/LoginView.vue'
import SyncView from '@/views/SyncView.vue'
import GitHubView from '@/views/GitHubView.vue'
import ConfigView from '@/views/ConfigView.vue'
import { PERM_SYNC, PERM_IMAGES, PERM_GITHUB, PERM_CONFIG, PERM_USERS, PERM_ROLES } from '@/constants/permissions'

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
    meta: { title: '镜像同步', requiredPermission: PERM_SYNC }
  },
  {
    path: '/images',
    name: 'ImagesManage',
    component: () => import('@/views/ImagesManageView.vue'),
    meta: { title: '镜像管理', requiredPermission: PERM_IMAGES }
  },
  {
    path: '/images/:acrId/:repoName/tags',
    name: 'ImageTags',
    component: () => import('@/views/ImageTagsView.vue'),
    meta: { title: 'Tag 列表', requiredPermission: PERM_IMAGES }
  },
  {
    path: '/github',
    name: 'GitHub',
    component: GitHubView,
    meta: { title: 'GitHub Actions', requiredPermission: PERM_GITHUB }
  },
  {
    path: '/config',
    name: 'Config',
    component: ConfigView,
    meta: { title: '系统配置', requiredPermission: PERM_CONFIG }
  },
  {
    path: '/users',
    name: 'UserManage',
    component: () => import('@/views/UserManageView.vue'),
    meta: { title: '用户管理', requiredPermission: PERM_USERS }
  },
  {
    path: '/roles',
    name: 'RoleManage',
    component: () => import('@/views/RoleManageView.vue'),
    meta: { title: '角色管理', requiredPermission: PERM_ROLES }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

function getUserPermissions() {
  const userStr = localStorage.getItem('user') || sessionStorage.getItem('user')
  const user = userStr ? JSON.parse(userStr) : null
  return user?.permissions || []
}

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

  if (to.meta.requiredPermission) {
    const permissions = getUserPermissions()
    if (!permissions.includes(to.meta.requiredPermission)) {
      next('/sync')
      return
    }
  }

  next()
})

export default router
