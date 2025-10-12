<!--
/**
 * 主应用组件 (App.vue)
 * 
 * 功能说明：
 * - 应用程序的根组件和主要布局容器
 * - 提供全局导航和页面布局结构
 * - 管理路由导航和页面切换
 * - 定义应用的整体样式和主题
 * 
 * 布局结构：
 * - Header: 顶部导航栏，包含Logo和主菜单
 * - Main: 主内容区域，用于显示路由页面
 * - Footer: 底部信息栏，显示版权信息
 * 
 * 导航功能：
 * - 镜像同步：跳转到同步操作页面
 * - GitHub Actions：跳转到GitHub监控页面
 * - 响应式导航高亮显示
 * 
 * 技术栈：
 * - Vue 3 Composition API
 * - Element Plus UI组件库
 * - Vue Router 路由管理
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */
-->

<template>
  <div id="app">
    <el-container class="layout-container">
      <!-- 顶部导航栏 -->
      <el-header class="header">
        <div class="header-content">
          <div class="logo">
            <el-icon><Box /></el-icon>
            <span>Docker镜像同步平台</span>
          </div>
          <el-menu
            :default-active="activeIndex"
            class="header-menu"
            mode="horizontal"
            @select="handleSelect"
          >
            <el-menu-item index="/sync">镜像同步</el-menu-item>
            <el-menu-item index="/github">GitHub Actions</el-menu-item>
          </el-menu>
        </div>
      </el-header>

      <!-- 主要内容区域 -->
      <el-main class="main-content">
        <router-view />
      </el-main>

      <!-- 底部 -->
      <el-footer class="footer">
        <div class="footer-content">
          <span>© 2024 Docker镜像同步平台 - 基于Vue 3 + Go开发</span>
        </div>
      </el-footer>
    </el-container>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Box } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

const activeIndex = computed(() => route.path)

const handleSelect = (key) => {
  console.log('Navigation triggered:', key)
  console.log('Current route:', route.path)
  try {
    router.push(key)
    console.log('Navigation successful')
  } catch (error) {
    console.error('Navigation error:', error)
  }
}
</script>

<style scoped>
.layout-container {
  min-height: 100vh;
}

.header {
  background-color: #fff;
  border-bottom: 1px solid #e4e7ed;
  padding: 0;
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 20px;
}

.logo {
  display: flex;
  align-items: center;
  font-size: 20px;
  font-weight: bold;
  color: #409eff;
}

.logo .el-icon {
  margin-right: 8px;
  font-size: 24px;
}

.header-menu {
  border-bottom: none;
}

.main-content {
  background-color: #f5f7fa;
  min-height: calc(100vh - 120px);
  padding: 20px;
}

.footer {
  background-color: #fff;
  border-top: 1px solid #e4e7ed;
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.footer-content {
  color: #909399;
  font-size: 14px;
}
</style>

<style>
body {
  margin: 0;
  font-family: 'Helvetica Neue', Helvetica, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', '微软雅黑', Arial, sans-serif;
}

#app {
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
</style>