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
        <AliyunConfigForm v-if="activeTab === 'aliyun'" />

        <div v-if="activeTab === 'system'" class="system-section">
          <div class="system-card">
            <div class="system-card-header">
              <el-icon class="system-card-icon"><Setting /></el-icon>
              <span>系统设置</span>
            </div>
            <el-form label-width="120px" class="system-form">
              <el-form-item label="日志级别">
                <el-select v-model="systemConfig.logLevel" placeholder="选择日志级别">
                  <el-option label="DEBUG" value="debug" />
                  <el-option label="INFO" value="info" />
                  <el-option label="WARN" value="warn" />
                  <el-option label="ERROR" value="error" />
                </el-select>
              </el-form-item>
              <el-form-item label="同步间隔">
                <el-input-number
                  v-model="systemConfig.syncInterval"
                  :min="1"
                  :max="1440"
                  controls-position="right"
                />
                <span class="form-suffix">分钟</span>
              </el-form-item>
              <el-form-item label="最大并发数">
                <el-input-number
                  v-model="systemConfig.maxConcurrency"
                  :min="1"
                  :max="10"
                  controls-position="right"
                />
              </el-form-item>
            </el-form>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Setting, Connection, Cloudy } from '@element-plus/icons-vue'
import GitConfigForm from '@/components/GitConfigForm.vue'
import AliyunConfigForm from '@/components/AliyunConfigForm.vue'
import { systemAPI } from '@/api'

const activeTab = ref('git')

const tabs = [
  { key: 'git', label: 'Git 配置', icon: Connection },
  { key: 'aliyun', label: '阿里云配置', icon: Cloudy },
  { key: 'system', label: '系统设置', icon: Setting },
]

const systemConfig = ref({
  logLevel: 'info',
  syncInterval: 60,
  maxConcurrency: 3
})

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
  background: var(--color-bg-muted);
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
