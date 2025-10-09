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

      <!-- 同步模式选择 -->
      <el-tabs v-model="activeTab" class="sync-tabs">
        <el-tab-pane label="单个同步" name="single">
          <!-- 单个同步表单 -->
          <SingleSyncForm @success="handleSingleSyncSuccess" />
        </el-tab-pane>
        <el-tab-pane label="批量同步" name="batch">
          <!-- 批量同步表单 -->
          <BatchSyncForm @success="handleBatchSyncSuccess" />
        </el-tab-pane>
        <el-tab-pane 
          v-if="currentBatchTask" 
          label="批量状态" 
          name="batch-status"
        >
          <!-- 批量同步状态 -->
          <BatchSyncStatus 
            :task-id="currentBatchTask.task_id" 
            @clear="handleBatchTaskClear"
          />
        </el-tab-pane>
      </el-tabs>
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
        <el-table-column prop="target_image" label="目标镜像" min-width="200">
          <template #default="scope">
            <div class="image-cell">
              <span class="image-text">{{ scope.row.target_image }}</span>
              <el-button 
                type="text" 
                size="small" 
                @click="copyToClipboard(scope.row.target_image)"
                class="copy-btn"
              >
                <el-icon><DocumentCopy /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="architecture" label="架构" width="100">
          <template #default="scope">
            <el-tag 
              :type="scope.row.architecture === 'arm64' ? 'warning' : 'primary'"
              size="small"
            >
              {{ scope.row.architecture }}
            </el-tag>
          </template>
        </el-table-column>
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

      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { 
  Refresh, 
  Clock, 
  Check, 
  Close, 
  Warning,
  Link,
  DocumentCopy
} from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'
import SingleSyncForm from '@/components/SingleSyncForm.vue'
import BatchSyncForm from '@/components/BatchSyncForm.vue'
import BatchSyncStatus from '@/components/BatchSyncStatus.vue'
import dayjs from 'dayjs'

// 状态管理
const syncStore = useSyncStore()

// 响应式数据
const refreshing = ref(false)
const historyLoading = ref(false)
const activeTab = ref('single')

// 当前批量任务
const currentBatchTask = ref(null)

// 处理单个同步成功
const handleSingleSyncSuccess = (task) => {
  // 确保currentTask已经设置（通常在submitSync中已经设置）
  if (task && !syncStore.currentTask) {
    syncStore.currentTask = task
  }
  startStatusPolling()
  loadRecentHistory()
}

// 处理批量同步成功
const handleBatchSyncSuccess = (task) => {
  currentBatchTask.value = task
  activeTab.value = 'batch-status'
  loadRecentHistory()
}

// 处理批量任务清除
const handleBatchTaskClear = () => {
  currentBatchTask.value = null
  activeTab.value = 'batch'
}

// 最近历史记录
const recentHistory = computed(() => {
  // 后端已经返回了完整的数据，包括source_image和target_image，直接使用即可
  return syncStore.syncHistory.slice(0, 5)
})



// 重试当前任务
const retryCurrentTask = async () => {
  if (!syncStore.currentTask) return
  
  try {
    // 从当前任务的images中获取镜像信息
    const images = syncStore.currentTask.images || []
    if (images.length === 0) {
      ElMessage.error('无法获取任务镜像信息')
      return
    }
    
    const syncData = {
      images: images
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
  console.log('Loading recent history...')
  historyLoading.value = true
  try {
    const result = await syncStore.loadSyncHistory({ page: 1, page_size: 5 })
    console.log('Recent history loaded:', result)
    console.log('Sync history data:', syncStore.syncHistory)
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
    running: 'warning',   // 对应后端的 running
    completed: 'success', // 对应后端的 completed
    failed: 'danger'
  }
  return statusMap[status] || 'info'
}

const getStatusText = (status) => {
  const statusMap = {
    pending: '等待中',
    running: '同步中',   // 对应后端的 running
    completed: '成功',   // 对应后端的 completed
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

// 复制到剪贴板
const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
  }
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

.image-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.image-name {
  flex: 1;
  word-break: break-all;
}

.task-actions {
  margin-top: 16px;
  text-align: right;
}

.task-actions .el-button {
  margin-left: 8px;
}
</style>