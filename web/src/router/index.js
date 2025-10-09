import { createRouter, createWebHistory } from 'vue-router'
import SyncView from '@/views/SyncView.vue'
import ImagesView from '@/views/ImagesView.vue'
import GitHubView from '@/views/GitHubView.vue'

const routes = [
  {
    path: '/',
    redirect: '/sync'
  },
  {
    path: '/sync',
    name: 'Sync',
    component: SyncView,
    meta: {
      title: '镜像同步'
    }
  },
  {
    path: '/images',
    name: 'Images',
    component: ImagesView,
    meta: {
      title: '镜像列表'
    }
  },
  {
    path: '/github',
    name: 'GitHub',
    component: GitHubView,
    meta: {
      title: 'GitHub Actions'
    }
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
  next()
})

export default router