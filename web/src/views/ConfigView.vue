<!--
  系统配置页面
  
  功能说明：
  - 提供Git仓库类型选择功能
  - 支持在Gitee和GitHub之间切换
  - 实时保存配置更改
  - 显示当前配置状态
  
  组件特性：
  - 响应式设计，适配不同屏幕尺寸
  - 友好的用户界面和交互体验
  - 完整的错误处理和状态反馈
  - 自动加载和保存配置
  
  @author Docker Image Sync Platform
  @version 1.0.0
-->

<template>
  <div class="config-container">
    <div class="config-header">
      <h1 class="page-title">系统配置</h1>
      <p class="page-description">管理系统的Git仓库和镜像同步配置</p>
    </div>

    <div class="config-content">
      <!-- 配置导航标签 -->
      <el-tabs v-model="activeTab" class="config-tabs">
        <el-tab-pane label="Git配置" name="git">
          <GitConfigForm />
        </el-tab-pane>
        
        <el-tab-pane label="阿里云配置" name="aliyun">
          <AliyunConfigForm />
        </el-tab-pane>
        
        <el-tab-pane label="系统设置" name="system">
          <div class="system-config">
            <el-card class="config-card" shadow="hover">
              <template #header>
                <div class="card-header">
                  <el-icon class="header-icon"><Setting /></el-icon>
                  <span class="header-title">系统设置</span>
                </div>
              </template>
              
              <div class="config-section">
                <el-form label-width="120px">
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
                    <span style="margin-left: 8px; color: #606266;">分钟</span>
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
            </el-card>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Setting, Monitor, Tools } from '@element-plus/icons-vue'
import GitConfigForm from '@/components/GitConfigForm.vue'
import AliyunConfigForm from '@/components/AliyunConfigForm.vue'
import { systemAPI } from '@/api'

// ====================================================================
// 响应式数据定义
// ====================================================================

const loading = ref(false)
const lastSaved = ref('')
const gitRepositoryType = ref('gitee')
const activeTab = ref('git')

const systemConfig = ref({
  logLevel: 'info',
  syncInterval: 60,
  maxConcurrency: 3
})

// ====================================================================
// 生命周期钩子
// ====================================================================

onMounted(() => {
  loadGitRepositoryConfig()
})

// ====================================================================
// 方法定义
// ====================================================================

/**
 * 加载Git仓库配置
 */
const loadGitRepositoryConfig = async () => {
  try {
    loading.value = true
    const response = await systemAPI.getGitRepositoryConfig()
    
    if (response.status === 'success') {
      gitRepositoryType.value = response.data.repository_type
    } else {
      ElMessage.error('加载配置失败：' + response.message)
    }
  } catch (error) {
    console.error('加载Git仓库配置失败:', error)
    ElMessage.error('加载配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

/**
 * 处理仓库类型变更
 */
const handleRepositoryTypeChange = async (newType) => {
  try {
    loading.value = true
    const response = await systemAPI.updateGitRepositoryConfig(newType)
    
    if (response.status === 'success') {
      updateLastSaved()
      ElMessage.success(`Git仓库类型已更新为 ${newType === 'gitee' ? 'Gitee' : 'GitHub'}`)
    } else {
      ElMessage.error('保存配置失败：' + response.message)
      // 恢复原值
      await loadGitRepositoryConfig()
    }
  } catch (error) {
    console.error('更新Git仓库配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
    // 恢复原值
    await loadGitRepositoryConfig()
  } finally {
    loading.value = false
  }
}

/**
 * 更新最后保存时间
 */
const updateLastSaved = () => {
  const now = new Date()
  lastSaved.value = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}
</script>

<style scoped>
/* ====================================================================
   页面整体布局样式
   ==================================================================== */

.config-container {
  padding: 24px;
  background-color: #f5f7fa;
  min-height: 100vh;
}

.config-header {
  margin-bottom: 24px;
}

.page-title {
  font-size: 28px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px 0;
}

.page-description {
  font-size: 14px;
  color: #606266;
  margin: 0;
  line-height: 1.6;
}

/* ====================================================================
   配置内容区域样式
   ==================================================================== */

.config-content {
  background-color: #fff;
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
}

.config-tabs {
  --el-tabs-header-height: 48px;
}

.config-tabs :deep(.el-tabs__header) {
  margin-bottom: 24px;
}

.config-tabs :deep(.el-tabs__nav-wrap) {
  background-color: #f8f9fa;
  border-radius: 8px;
  padding: 4px;
}

.config-tabs :deep(.el-tabs__item) {
  border-radius: 6px;
  margin: 0 2px;
  transition: all 0.3s ease;
}

.config-tabs :deep(.el-tabs__item.is-active) {
  background-color: #409eff;
  color: #fff;
}

.config-tabs :deep(.el-tabs__item:hover) {
  background-color: #ecf5ff;
  color: #409eff;
}

.config-tabs :deep(.el-tabs__item.is-active:hover) {
  background-color: #337ecc;
  color: #fff;
}

/* ====================================================================
   系统配置样式
   ==================================================================== */

.system-config {
  max-width: 600px;
}

.config-card {
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  transition: all 0.3s ease;
}

.config-card:hover {
  border-color: #409eff;
  transform: translateY(-2px);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #303133;
}

.header-icon {
  font-size: 18px;
  color: #409eff;
}

.header-title {
  font-size: 16px;
}

.config-section {
  padding: 4px 0;
}

/* ====================================================================
   响应式设计
   ==================================================================== */

@media (max-width: 768px) {
  .config-container {
    padding: 16px;
  }
  
  .config-content {
    padding: 16px;
  }
  
  .page-title {
    font-size: 24px;
  }
  
  .config-tabs :deep(.el-tabs__nav-wrap) {
    padding: 2px;
  }
  
  .config-tabs :deep(.el-tabs__item) {
    font-size: 14px;
    padding: 0 12px;
  }
}

@media (max-width: 480px) {
  .config-container {
    padding: 12px;
  }
  
  .config-content {
    padding: 12px;
  }
  
  .page-title {
    font-size: 20px;
  }
}
</style>