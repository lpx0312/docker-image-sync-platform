/**
 * Vue Router 路由配置
 * 
 * 功能说明：
 * - 定义应用程序的路由规则和页面导航
 * - 配置页面组件的映射关系
 * - 设置路由元信息和页面标题
 * - 提供路由守卫和导航控制
 * 
 * 路由结构：
 * - /: 根路径，重定向到镜像同步页面
 * - /sync: 镜像同步页面，主要功能入口
 * - /github: GitHub Actions监控页面
 * 
 * 路由特性：
 * - 使用HTML5 History模式
 * - 自动设置页面标题
 * - 支持路由元信息配置
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */

import { createRouter, createWebHistory } from 'vue-router'
import SyncView from '@/views/SyncView.vue'
import GitHubView from '@/views/GitHubView.vue'
import ConfigView from '@/views/ConfigView.vue'

/**
 * 路由配置数组
 * 定义所有可访问的页面路径和对应组件
 */
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
    path: '/github',
    name: 'GitHub',
    component: GitHubView,
    meta: {
      title: 'GitHub Actions'
    }
  },
  {
    path: '/config',
    name: 'Config',
    component: ConfigView,
    meta: {
      title: '系统配置'
    }
  }
]

/**
 * 创建路由实例
 * 使用HTML5 History模式，提供更好的用户体验
 */
const router = createRouter({
  history: createWebHistory(), // 使用HTML5 History API
  routes // 路由配置
})

/**
 * 全局前置路由守卫
 * 在每次路由跳转前执行，用于设置页面标题等全局操作
 * 
 * @param {Object} to - 即将进入的目标路由对象
 * @param {Object} from - 当前导航正要离开的路由对象
 * @param {Function} next - 调用该方法来resolve这个钩子
 */
router.beforeEach((to, from, next) => {
  // 根据路由元信息设置页面标题
  if (to.meta.title) {
    document.title = `${to.meta.title} - Docker镜像同步平台`
  }
  next() // 继续路由导航
})

export default router