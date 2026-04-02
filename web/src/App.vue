<template>
  <div id="app">
    <!-- 登录页不显示布局 -->
    <router-view v-if="!showLayout" />

    <!-- 主布局 -->
    <el-container v-else class="layout-container">
      <el-header class="header">
        <div class="header-content">
          <div class="logo">
            <el-icon><Box /></el-icon>
            <span>Docker镜像同步平台</span>
          </div>
          <div class="header-right">
            <el-menu
              :default-active="activeIndex"
              class="header-menu"
              mode="horizontal"
              @select="handleSelect"
            >
              <el-menu-item index="/sync">镜像同步</el-menu-item>
              <el-menu-item index="/github">GitHub Actions</el-menu-item>
              <el-menu-item index="/config">系统配置</el-menu-item>
              <el-menu-item v-if="authStore.isAdmin" index="/users">用户管理</el-menu-item>
            </el-menu>
            <el-dropdown class="user-dropdown" @command="handleUserCommand">
              <span class="user-info">
                <el-icon><UserFilled /></el-icon>
                <span class="user-name">{{ authStore.username }}</span>
                <el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="password">
                    <el-icon><Key /></el-icon>修改密码
                  </el-dropdown-item>
                  <el-dropdown-item divided command="logout">
                    <el-icon><SwitchButton /></el-icon>退出登录
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </el-header>

      <el-main class="main-content">
        <router-view />
      </el-main>

      <el-footer class="footer">
        <div class="footer-content">
          <span>© 2024 Docker镜像同步平台 - 基于Vue 3 + Go开发</span>
        </div>
      </el-footer>
    </el-container>

    <!-- 修改密码对话框 -->
    <ChangePasswordDialog
      v-model:visible="showChangePassword"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Box, UserFilled, ArrowDown, Key, SwitchButton } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const showChangePassword = ref(false)
const activeIndex = computed(() => route.path)
const showLayout = computed(() => route.name !== 'Login' && authStore.isLoggedIn)

const handleSelect = (key) => {
  router.push(key)
}

const handleUserCommand = (command) => {
  if (command === 'password') {
    showChangePassword.value = true
  } else if (command === 'logout') {
    ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }).then(() => {
      authStore.logout()
      router.push('/login')
    }).catch(() => {})
  }
}

function onActivity() {
  authStore.updateActivityTime()
}

onMounted(() => {
  window.addEventListener('mousemove', onActivity)
  window.addEventListener('keydown', onActivity)
  window.addEventListener('click', onActivity)
})

onUnmounted(() => {
  window.removeEventListener('mousemove', onActivity)
  window.removeEventListener('keydown', onActivity)
  window.removeEventListener('click', onActivity)
})
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
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 20px;
}

.logo {
  display: flex;
  align-items: center;
  font-size: 20px;
  font-weight: bold;
  color: #409eff;
  white-space: nowrap;
}

.logo .el-icon {
  margin-right: 8px;
  font-size: 24px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-menu {
  border-bottom: none;
}

.user-dropdown {
  cursor: pointer;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  color: #606266;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background-color 0.2s;
}

.user-info:hover {
  background-color: #f5f7fa;
}

.user-name {
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
