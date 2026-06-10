<template>
  <div class="config-view">
    <div class="page-header">
      <h1 class="page-title">系统配置</h1>
      <p class="page-desc">管理 Git 仓库和镜像同步配置</p>
    </div>

    <div class="config-panel">
      <div class="tab-nav">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="tab-btn"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          <component :is="tab.icon" class="tab-icon" />
          <span>{{ tab.label }}</span>
        </button>
      </div>

      <div class="tab-content">
        <GitConfigForm v-if="activeTab === 'git'" />
        <AcrRegistryConfigForm v-if="activeTab === 'acr'" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, Cloudy } from '@element-plus/icons-vue'
import GitConfigForm from '@/components/GitConfigForm.vue'
import AcrRegistryConfigForm from '@/components/AcrRegistryConfigForm.vue'
import { systemAPI } from '@/api'

const activeTab = ref('git')

const tabs = [
  { key: 'git', label: 'Git 配置', icon: Connection },
  { key: 'acr', label: 'ACR 配置', icon: Cloudy },
]

const loading = ref(false)
const gitRepositoryType = ref('gitee')

onMounted(() => {
  loadGitRepositoryConfig()
})

const loadGitRepositoryConfig = async () => {
  try {
    loading.value = true
    const response = await systemAPI.getGitRepositoryConfig()
    if (response.status === 'success') {
      gitRepositoryType.value = response.data.repository_type
    }
  } catch (error) {
    ElMessage.error('加载配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.config-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--space-lg);
}

.page-title {
  margin: 0 0 4px;
  font-size: 24px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.page-desc {
  margin: 0;
  font-size: 14px;
  color: var(--color-text-muted);
}

.system-hint {
  line-height: 1.6;
}

.system-hint code {
  font-size: 12px;
  padding: 0 4px;
  background: var(--color-bg-muted, #f4f4f5);
  border-radius: 4px;
}

/* ── Tab Panel ── */
.config-panel {
  background: var(--color-bg-card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}

.tab-nav {
  display: flex;
  gap: var(--space-xs);
  padding: var(--space-md) var(--space-lg) 0;
  border-bottom: 1px solid var(--color-border-light);
  background: var(--color-bg-card);
}

.tab-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 20px;
  border: none;
  background: transparent;
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  position: relative;
  bottom: -1px;
}

.tab-btn:hover {
  color: var(--color-primary);
  background: var(--color-primary-bg);
}

.tab-btn.active {
  color: var(--color-primary);
  background: var(--color-bg-card);
  border: 1px solid var(--color-border-light);
  border-bottom-color: var(--color-bg-card);
  font-weight: 600;
}

.tab-icon {
  width: 16px;
  height: 16px;
}

.tab-content {
  padding: var(--space-lg);
}

/* ── System Settings ── */
.system-section {
  max-width: 560px;
}

.system-card {
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.system-card-header {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  padding: var(--space-md) var(--space-lg);
  background: var(--color-bg-muted);
  border-bottom: 1px solid var(--color-border-light);
  font-weight: 600;
  font-size: 15px;
  color: var(--color-text-primary);
}

.system-card-icon {
  color: var(--color-primary);
  font-size: 18px;
}

.system-form {
  padding: var(--space-lg);
}

.form-suffix {
  margin-left: var(--space-sm);
  color: var(--color-text-secondary);
  font-size: 14px;
}

/* ── Responsive ── */
@media (max-width: 640px) {
  .tab-nav {
    padding: var(--space-sm) var(--space-md) 0;
    overflow-x: auto;
  }

  .tab-btn {
    padding: 8px 14px;
    font-size: 13px;
    white-space: nowrap;
  }

  .tab-content {
    padding: var(--space-md);
  }
}
</style>
