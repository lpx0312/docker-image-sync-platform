/**
 * Vue 3 应用程序入口文件
 * 
 * 功能说明：
 * - 创建和配置Vue应用实例
 * - 注册全局插件和组件库
 * - 配置状态管理和路由
 * - 设置国际化和主题
 * 
 * 依赖库：
 * - Vue 3: 核心框架
 * - Pinia: 状态管理
 * - Element Plus: UI组件库
 * - Vue Router: 路由管理
 * 
 * 配置特性：
 * - 中文本地化支持
 * - 全局图标组件注册
 * - 响应式状态管理
 * - 单页面应用路由
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */

// Vue 3 核心
import { createApp } from 'vue'
// Pinia 状态管理
import { createPinia } from 'pinia'
// Element Plus UI组件库
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
// Element Plus 图标库
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
// Element Plus 中文语言包
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'

// 应用组件和路由
import App from './App.vue'
import router from './router'

/**
 * 创建Vue应用实例
 */
const app = createApp(App)

/**
 * 全局注册Element Plus图标组件
 * 使所有图标可以在模板中直接使用
 */
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

/**
 * 注册应用插件和配置
 */
app.use(createPinia()) // 状态管理
app.use(router) // 路由管理
app.use(ElementPlus, {
  locale: zhCn, // 设置为中文本地化
})

/**
 * 挂载应用到DOM
 */
app.mount('#app')