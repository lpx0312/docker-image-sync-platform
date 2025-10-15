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
    <!-- 页面标题区域 -->
    <div class="page-header">
      <h1 class="page-title">
        <el-icon><Setting /></el-icon>
        系统配置
      </h1>
      <p class="page-description">
        配置系统的基本参数和行为设置
      </p>
    </div>

    <!-- 配置内容区域 -->
    <div class="config-content">
      <!-- Git仓库配置卡片 -->
      <el-card class="config-card" shadow="hover">
        <template #header>
          <div class="card-header">
            <el-icon class="header-icon"><Connection /></el-icon>
            <span class="header-title">Git仓库配置</span>
          </div>
        </template>

        <div class="config-section">
          <div class="section-description">
            <p>选择系统使用的Git仓库类型。更改此设置将影响所有新的镜像同步任务。</p>
          </div>

          <div class="config-item">
            <label class="config-label">Git仓库类型</label>
            <el-radio-group 
              v-model="gitRepositoryType" 
              class="repository-radio-group"
              @change="handleRepositoryTypeChange"
              :disabled="loading"
            >
              <el-radio value="gitee" class="repository-radio">
                <div class="radio-content">
                  <div class="radio-header">
                    <span class="radio-title">Gitee</span>
                    <el-tag v-if="gitRepositoryType === 'gitee'" type="success" size="small">当前选择</el-tag>
                  </div>
                  <div class="radio-description">
                    使用Gitee作为Git仓库，适合国内用户，访问速度更快
                  </div>
                </div>
              </el-radio>
              
              <el-radio value="github" class="repository-radio">
                <div class="radio-content">
                  <div class="radio-header">
                    <span class="radio-title">GitHub</span>
                    <el-tag v-if="gitRepositoryType === 'github'" type="success" size="small">当前选择</el-tag>
                  </div>
                  <div class="radio-description">
                    使用GitHub作为Git仓库，全球最大的代码托管平台
                  </div>
                </div>
              </el-radio>
            </el-radio-group>
          </div>

          <!-- 配置状态显示 -->
          <div class="config-status" v-if="lastSaved">
            <el-icon class="status-icon"><CircleCheck /></el-icon>
            <span class="status-text">配置已保存 - {{ lastSaved }}</span>
          </div>
        </div>
      </el-card>

      <!-- 其他配置卡片可以在这里添加 -->
      <el-card class="config-card" shadow="hover">
        <template #header>
          <div class="card-header">
            <el-icon class="header-icon"><InfoFilled /></el-icon>
            <span class="header-title">配置说明</span>
          </div>
        </template>

        <div class="config-section">
          <el-alert
            title="重要提示"
            type="info"
            :closable="false"
            show-icon
          >
            <template #default>
              <ul class="alert-list">
                <li>更改Git仓库类型后，新的同步任务将使用新的仓库类型</li>
                <li>正在进行的同步任务不会受到影响</li>
                <li>请确保目标仓库已正确配置访问权限</li>
                <li>配置更改会立即生效，无需重启服务</li>
              </ul>
            </template>
          </el-alert>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Setting, Connection, CircleCheck, InfoFilled } from '@element-plus/icons-vue'
import { systemAPI } from '@/api'

// ====================================================================
// 响应式数据定义
// ====================================================================

const gitRepositoryType = ref('gitee') // Git仓库类型
const loading = ref(false) // 加载状态
const lastSaved = ref('') // 最后保存时间

// ====================================================================
// 生命周期钩子
// ====================================================================

/**
 * 组件挂载时加载配置
 */
onMounted(() => {
  loadGitRepositoryConfig()
})

// ====================================================================
// 方法定义
// ====================================================================

/**
 * 加载Git仓库配置
 * 从后端API获取当前的Git仓库类型设置
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
 * 当用户选择不同的仓库类型时，自动保存到后端
 * 
 * @param {string} newType - 新选择的仓库类型
 */
const handleRepositoryTypeChange = async (newType) => {
  try {
    loading.value = true
    const response = await systemAPI.updateGitRepositoryConfig(newType)
    
    if (response.status === 'success') {
      // 更新最后保存时间
      const now = new Date()
      lastSaved.value = now.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      })
      
      ElMessage.success(`Git仓库类型已更新为 ${newType === 'gitee' ? 'Gitee' : 'GitHub'}`)
    } else {
      ElMessage.error('保存配置失败：' + response.message)
      // 恢复原来的值
      await loadGitRepositoryConfig()
    }
  } catch (error) {
    console.error('更新Git仓库配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
    // 恢复原来的值
    await loadGitRepositoryConfig()
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* ====================================================================
   页面整体布局样式
   ==================================================================== */

.config-container {
  padding: 24px;
  background-color: #f5f7fa;
  min-height: calc(100vh - 60px);
}

.page-header {
  margin-bottom: 24px;
}

.page-title {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 28px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px 0;
}

.page-description {
  color: #606266;
  font-size: 14px;
  margin: 0;
}

.config-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* ====================================================================
   配置卡片样式
   ==================================================================== */

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

.section-description {
  margin-bottom: 20px;
}

.section-description p {
  color: #606266;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}

/* ====================================================================
   配置项样式
   ==================================================================== */

.config-item {
  margin-bottom: 24px;
}

.config-label {
  display: block;
  font-weight: 600;
  color: #303133;
  margin-bottom: 12px;
  font-size: 14px;
}

.repository-radio-group {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.repository-radio {
  border: 2px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
  margin: 0;
  transition: all 0.3s ease;
  background-color: #fff;
}

.repository-radio:hover {
  border-color: #409eff;
  background-color: #f0f9ff;
}

.repository-radio.is-checked {
  border-color: #409eff;
  background-color: #f0f9ff;
}

.radio-content {
  margin-left: 8px;
}

.radio-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.radio-title {
  font-weight: 600;
  color: #303133;
  font-size: 16px;
}

.radio-description {
  color: #606266;
  font-size: 13px;
  line-height: 1.5;
}

/* ====================================================================
   配置状态样式
   ==================================================================== */

.config-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background-color: #f0f9ff;
  border: 1px solid #b3d8ff;
  border-radius: 6px;
  margin-top: 16px;
}

.status-icon {
  color: #67c23a;
  font-size: 16px;
}

.status-text {
  color: #409eff;
  font-size: 13px;
}

/* ====================================================================
   提示信息样式
   ==================================================================== */

.alert-list {
  margin: 0;
  padding-left: 20px;
  color: #606266;
}

.alert-list li {
  margin-bottom: 8px;
  line-height: 1.6;
}

.alert-list li:last-child {
  margin-bottom: 0;
}

/* ====================================================================
   响应式设计
   ==================================================================== */

@media (max-width: 768px) {
  .config-container {
    padding: 16px;
  }
  
  .page-title {
    font-size: 24px;
  }
  
  .repository-radio {
    padding: 12px;
  }
  
  .radio-title {
    font-size: 15px;
  }
  
  .radio-description {
    font-size: 12px;
  }
}
</style>