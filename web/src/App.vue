<template>
  <div id="app">
    <router-view v-if="!showLayout" />

    <el-container v-else class="layout-container">
      <header class="app-header">
        <div class="header-inner">
          <div class="logo" @click="router.push('/sync')">
            <div class="logo-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="22" height="22">
                <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
                <line x1="12" y1="22.08" x2="12" y2="12"/>
              </svg>
            </div>
            <span class="logo-text">Docker 镜像同步</span>
          </div>

          <nav class="nav-links">
            <router-link
              v-for="item in navItems"
              :key="item.path"
              :to="item.path"
              class="nav-item"
              :class="{ active: isActiveRoute(item.path) }"
            >
              <component :is="item.icon" class="nav-icon" />
              <span>{{ item.label }}</span>
            </router-link>
          </nav>

          <div class="header-actions">
            <el-dropdown trigger="click" @command="handleUserCommand">
              <button class="user-btn">
                <div class="user-avatar">
                  {{ (authStore.username || 'U').charAt(0).toUpperCase() }}
                </div>
                <span class="user-name">{{ authStore.username }}</span>
                <el-icon class="dropdown-arrow"><ArrowDown /></el-icon>
              </button>
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
      </header>

      <el-main class="main-content">
        <transition name="page" mode="out-in">
          <router-view />
        </transition>
      </el-main>

      <footer class="app-footer">
        <span>&copy; {{ new Date().getFullYear() }} Docker 镜像同步平台</span>
      </footer>
    </el-container>

    <ChangePasswordDialog v-model:visible="showChangePassword" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ArrowDown, Key, SwitchButton, Box, Connection, Setting, User } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const showChangePassword = ref(false)
const showLayout = computed(() => route.name !== 'Login' && authStore.isLoggedIn)

const allNavItems = [
  { path: '/sync', label: '镜像同步', icon: Box, permission: 'sync' },
  { path: '/github', label: 'GitHub Actions', icon: Connection, permission: 'github' },
  { path: '/config', label: '系统配置', icon: Setting, permission: 'config' },
  { path: '/users', label: '用户管理', icon: User, permission: 'users' },
]

const navItems = computed(() => {
  return allNavItems.filter(item => authStore.hasPermission(item.permission))
})

const isActiveRoute = (path) => {
  return route.path === path || route.path.startsWith(path + '/')
}

const handleUserCommand = (command) => {
  if (command === 'password') {
    showChangePassword.value = true
  } else if (command === 'logout') {
    ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }).then(async () => {
      await authStore.logout()
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
  display: flex;
  flex-direction: column;
}

.app-header {
  position: sticky;
  top: 0;
  z-index: 100;
  background: var(--color-bg-header);
  border-bottom: 1px solid var(--color-border);
  backdrop-filter: blur(12px);
}

.header-inner {
  display: flex;
  align-items: center;
  height: 56px;
  max-width: var(--max-width);
  margin: 0 auto;
  padding: 0 var(--space-lg);
  gap: var(--space-xl);
}

.logo {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  cursor: pointer;
  flex-shrink: 0;
  text-decoration: none;
  transition: opacity var(--transition-fast);
}

.logo:hover {
  opacity: 0.85;
}

.logo-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--color-primary), var(--color-primary-light));
  color: var(--color-text-inverse);
}

.logo-text {
  font-size: 16px;
  font-weight: 700;
  color: var(--color-text-primary);
  letter-spacing: -0.01em;
}

.nav-links {
  display: flex;
  align-items: center;
  gap: var(--space-xs);
  flex: 1;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: var(--radius-md);
  font-size: 14px;
  font-weight: 500;
  color: var(--color-text-secondary);
  text-decoration: none;
  transition: all var(--transition-fast);
  cursor: pointer;
  white-space: nowrap;
}

.nav-item:hover {
  color: var(--color-primary);
  background: var(--color-primary-bg);
}

.nav-item.active {
  color: var(--color-primary);
  background: var(--color-primary-bg);
  font-weight: 600;
}

.nav-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.header-actions {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.user-btn {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  padding: 4px 10px 4px 4px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  background: var(--color-bg-card);
  cursor: pointer;
  transition: all var(--transition-fast);
  outline: none;
}

.user-btn:hover {
  border-color: var(--color-primary-lighter);
  background: var(--color-primary-bg);
}

.user-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--color-primary), var(--color-primary-light));
  color: var(--color-text-inverse);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
}

.user-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
  max-width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dropdown-arrow {
  font-size: 12px;
  color: var(--color-text-muted);
}

.main-content {
  flex: 1;
  background: var(--color-bg-page);
  padding: var(--space-lg);
  min-height: 0;
}

.app-footer {
  padding: var(--space-md) var(--space-lg);
  text-align: center;
  font-size: 13px;
  color: var(--color-text-muted);
  border-top: 1px solid var(--color-border-light);
  background: var(--color-bg-card);
}
</style>
