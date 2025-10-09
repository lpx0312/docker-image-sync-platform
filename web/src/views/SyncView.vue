<template>
  <div class="sync-view">
    <el-card class="sync-card">
      <template #header>
        <div class="card-header">
          <span>镜像同步</span>
          <el-button 
            type="primary" 
            :icon="Refresh" 
            @click="refreshStatus"
            :loading="refreshing"
            size="small"
          >
            刷新状态
          </el-button>
        </div>
      </template>

      <!-- 同步表单 -->
      <el-form 
        ref="syncFormRef" 
        :model="syncForm" 
        :rules="syncRules" 
        label-width="120px"
        :disabled="syncStore.hasCurrentTask && syncStore.taskStatus !== 'failed'"
      >
        <el-form-item label="源镜像地址" prop="sourceImage">
          <el-input
            v-model="syncForm.sourceImage"
            placeholder="例如: nginx:latest 或 docker.io/library/nginx:latest"
            clearable
          />
          <div class="form-tip">
            支持Docker Hub、Quay.io等公共镜像仓库的镜像
          </div>
        </el-form-item>

        <el-form-item label="目标标签" prop="targetTag">
          <el-input
            v-model="syncForm.targetTag"
            placeholder="例如: nginx:latest 或自定义标签"
            clearable
          />
          <div class="form-tip">
            留空则使用源镜像的标签，支持自定义标签
          </div>
        </el-form-item>

        <el-form-item label="同步说明" prop="description">
          <el-input
            v-model="syncForm.description"
            type="textarea"
            :rows="3"
            placeholder="可选：描述此次同步的目的或说明"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>

        <el-form-item>
          <el-button 
            type="primary" 
            @click="submitSync" 
            :loading="syncStore.loading"
            :disabled="syncStore.hasCurrentTask && syncStore.taskStatus !== 'failed'"
          >
            <el-icon><Upload /></el-icon>
            开始同步
          </el-button>
          <el-button @click="resetForm">
            <el-icon><RefreshLeft /></el-icon>
            重置
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 当前任务状态 -->
    <el-card v-if="syncStore.hasCurrentTask" class="status-card">
      <template #header>
        <div class="card-header">
          <span>当前同步任务</span>
          <el-tag :type="getStatusType(syncStore.taskStatus)">
            {{ getStatusText(syncStore.taskStatus) }}
          </el-tag>
        </div>
      </template>

      <div class="task-info">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="任务ID">
            {{ syncStore.currentTask.task_id }}
          </el-descriptions-item>
          <el-descriptions-item label="源镜像">
            {{ syncStore.currentTask.source_image }}
          </el-descriptions-item>
          <el-descriptions-item label="目标镜像">
            {{ syncStore.currentTask.target_image }}
          </el-descriptions-item>
          <el-descriptions-item label="创建时间">
            {{ formatTime(syncStore.currentTask.created_at) }}
          </el-descriptions-item>
          <el-descriptions-item label="GitHub Actions" v-if="syncStore.currentTask.github_run_url">
            <el-link :href="syncStore.currentTask.github_run_url" target="_blank" type="primary">
              查看运行详情
            </el-link>
          </el-descriptions-item>
          <el-descriptions-item label="同步说明" v-if="syncStore.currentTask.description">
            {{ syncStore.currentTask.description }}
          </el-descriptions-item>
        </el-descriptions>

        <!-- 进度条 -->
        <div class="progress-section" v-if="syncStore.taskStatus === 'syncing'">
          <el-progress 
            :percentage="getProgress()" 
            :status="getProgressStatus()"
            :stroke-width="8"
          />
          <div class="progress-text">
            {{ getProgressText() }}
          </div>
        </div>

        <!-- 错误信息 -->
        <el-alert
          v-if="syncStore.taskStatus === 'failed' && syncStore.currentTask.message"
          :title="syncStore.currentTask.message"
          type="error"
          :closable="false"
          show-icon
        />

        <!-- 成功信息 -->
        <el-alert
          v-if="syncStore.taskStatus === 'success'"
          title="镜像同步成功！"
          type="success"
          :closable="false"
          show-icon
        >
          <template #default>
            <div>镜像已成功同步到阿里云容器镜像服务</div>
            <div v-if="syncStore.currentTask.target_image">
              目标镜像：<el-tag>{{ syncStore.currentTask.target_image }}</el-tag>
            </div>
          </template>
        </el-alert>

        <!-- 操作按钮 -->
        <div class="task-actions">
          <el-button 
            v-if="syncStore.taskStatus === 'failed'" 
            type="primary" 
            @click="retryCurrentTask"
            :loading="syncStore.loading"
          >
            重试同步
          </el-button>
          <el-button 
            v-if="['success', 'failed'].includes(syncStore.taskStatus)" 
            @click="clearCurrentTask"
          >
            清除任务
          </el-button>
        </div>
      </div>
    </el-card>

    <!-- 最近同步历史 -->
    <el-card class="history-card">
      <template #header>
        <div class="card-header">
          <span>最近同步历史</span>
          <el-button 
            type="text" 
            @click="$router.push('/images')"
            size="small"
          >
            查看全部
          </el-button>
        </div>
      </template>

      <el-table 
        :data="recentHistory" 
        v-loading="historyLoading"
        empty-text="暂无同步记录"
      >
        <el-table-column prop="source_image" label="源镜像" min-width="200" />
        <el-table-column prop="target_image" label="目标镜像" min-width="200" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button 
              v-if="row.github_run_url" 
              type="text" 
              size="small"
              @click="openGitHubRun(row.github_run_url)"
            >
              查看详情
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Upload, RefreshLeft } from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'
import dayjs from 'dayjs'

const syncStore = useSyncStore()
const syncFormRef = ref()
const refreshing = ref(false)
const historyLoading = ref(false)

// 表单数据
const syncForm = reactive({
  sourceImage: '',
  targetTag: '',
  description: ''
})

// 表单验证规则
const syncRules = {
  sourceImage: [
    { required: true, message: '请输入源镜像地址', trigger: 'blur' },
    { 
      pattern: /^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$/, 
      message: '请输入有效的镜像地址格式', 
      trigger: 'blur' 
    }
  ]
}

// 最近历史记录
const recentHistory = computed(() => {
  return syncStore.syncHistory.slice(0, 5)
})

// 提交同步
const submitSync = async () => {
  try {
    await syncFormRef.value.validate()
    
    const syncData = {
      source_image: syncForm.sourceImage,
      target_tag: syncForm.targetTag || '',
      description: syncForm.description || ''
    }
    
    await syncStore.submitSync(syncData)
    ElMessage.success('同步任务已提交')
    
    // 开始轮询状态
    startStatusPolling()
    
  } catch (error) {
    if (error.errors) {
      // 表单验证错误
      return
    }
    ElMessage.error('提交同步任务失败')
  }
}

// 重置表单
const resetForm = () => {
  syncFormRef.value.resetFields()
  Object.assign(syncForm, {
    sourceImage: '',
    targetTag: '',
    description: ''
  })
}

// 重试当前任务
const retryCurrentTask = async () => {
  if (!syncStore.currentTask) return
  
  try {
    // 重新提交相同的任务
    const syncData = {
      source_image: syncStore.currentTask.source_image,
      target_tag: syncStore.currentTask.target_tag || '',
      description: syncStore.currentTask.description || ''
    }
    
    await syncStore.submitSync(syncData)
    ElMessage.success('重试任务已提交')
    startStatusPolling()
    
  } catch (error) {
    ElMessage.error('重试任务失败')
  }
}

// 清除当前任务
const clearCurrentTask = () => {
  syncStore.clearCurrentTask()
  stopStatusPolling()
}

// 状态轮询
let statusPollingTimer = null

const startStatusPolling = () => {
  if (!syncStore.currentTask) return
  
  statusPollingTimer = setInterval(async () => {
    try {
      await syncStore.getSyncStatus(syncStore.currentTask.task_id)
      
      // 如果任务完成，停止轮询
      if (['success', 'failed'].includes(syncStore.taskStatus)) {
        stopStatusPolling()
        // 刷新历史记录
        loadRecentHistory()
      }
    } catch (error) {
      console.error('轮询状态失败:', error)
    }
  }, 5000) // 每5秒轮询一次
}

const stopStatusPolling = () => {
  if (statusPollingTimer) {
    clearInterval(statusPollingTimer)
    statusPollingTimer = null
  }
}

// 刷新状态
const refreshStatus = async () => {
  if (!syncStore.currentTask) return
  
  refreshing.value = true
  try {
    await syncStore.getSyncStatus(syncStore.currentTask.task_id)
    ElMessage.success('状态已刷新')
  } catch (error) {
    ElMessage.error('刷新状态失败')
  } finally {
    refreshing.value = false
  }
}

// 加载最近历史
const loadRecentHistory = async () => {
  historyLoading.value = true
  try {
    await syncStore.loadSyncHistory({ page: 1, page_size: 5 })
  } catch (error) {
    console.error('加载历史记录失败:', error)
  } finally {
    historyLoading.value = false
  }
}

// 工具函数
const getStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    syncing: 'warning',
    success: 'success',
    failed: 'danger'
  }
  return statusMap[status] || 'info'
}

const getStatusText = (status) => {
  const statusMap = {
    pending: '等待中',
    syncing: '同步中',
    success: '成功',
    failed: '失败'
  }
  return statusMap[status] || '未知'
}

const getProgress = () => {
  if (!syncStore.currentTask) return 0
  
  const status = syncStore.taskStatus
  if (status === 'pending') return 10
  if (status === 'syncing') return 50
  if (status === 'success') return 100
  if (status === 'failed') return 100
  
  return 0
}

const getProgressStatus = () => {
  const status = syncStore.taskStatus
  if (status === 'failed') return 'exception'
  if (status === 'success') return 'success'
  return undefined
}

const getProgressText = () => {
  const status = syncStore.taskStatus
  if (status === 'pending') return '任务已提交，等待处理...'
  if (status === 'syncing') return '正在同步镜像，请稍候...'
  return ''
}

const formatTime = (time) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

const openGitHubRun = (url) => {
  window.open(url, '_blank')
}

// 生命周期
onMounted(() => {
  loadRecentHistory()
  
  // 如果有当前任务且正在进行中，开始轮询
  if (syncStore.hasCurrentTask && ['pending', 'syncing'].includes(syncStore.taskStatus)) {
    startStatusPolling()
  }
})

// 组件卸载时清理定时器
import { onUnmounted } from 'vue'
onUnmounted(() => {
  stopStatusPolling()
})
</script>

<style scoped>
.sync-view {
  max-width: 1000px;
  margin: 0 auto;
}

.sync-card,
.status-card,
.history-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.task-info {
  margin-top: 16px;
}

.progress-section {
  margin: 20px 0;
}

.progress-text {
  text-align: center;
  margin-top: 8px;
  color: #606266;
  font-size: 14px;
}

.task-actions {
  margin-top: 16px;
  text-align: right;
}

.task-actions .el-button {
  margin-left: 8px;
}
</style>