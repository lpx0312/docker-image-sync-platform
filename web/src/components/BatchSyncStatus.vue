<template>
  <div class="batch-sync-status">
    <el-card class="status-card">
      <template #header>
        <div class="card-header">
          <span>批量同步任务状态</span>
          <div class="header-actions">
            <el-tag :type="getStatusType(taskStatus)">
              {{ getStatusText(taskStatus) }}
            </el-tag>
            <el-button 
              type="primary" 
              size="small"
              :icon="Refresh" 
              @click="refreshStatus"
              :loading="refreshing"
            >
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <!-- 任务基本信息 -->
      <div class="task-overview">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="任务ID">
            {{ taskData.task_id }}
          </el-descriptions-item>
          <el-descriptions-item label="任务描述">
            {{ taskData.description }}
          </el-descriptions-item>
          <el-descriptions-item label="总镜像数">
            {{ taskData.total_images }}
          </el-descriptions-item>
          <el-descriptions-item label="并发数">
            {{ taskData.max_concurrent }}
          </el-descriptions-item>
          <el-descriptions-item label="已完成">
            <el-tag type="success">{{ taskData.completed_images }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="失败数">
            <el-tag type="danger">{{ taskData.failed_images }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="创建时间">
            {{ formatTime(taskData.created_at) }}
          </el-descriptions-item>
          <el-descriptions-item label="预计剩余时间" v-if="taskData.estimated_remaining_time">
            {{ taskData.estimated_remaining_time }}
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <!-- 进度条 -->
      <div class="progress-section">
        <div class="progress-header">
          <span>同步进度</span>
          <span class="progress-text">{{ taskData.progress?.toFixed(1) || 0 }}%</span>
        </div>
        <el-progress 
          :percentage="taskData.progress || 0" 
          :status="getProgressStatus()"
          :stroke-width="12"
          :show-text="false"
        />
        <div class="progress-stats">
          <span>已完成: {{ taskData.completed_images || 0 }}</span>
          <span>失败: {{ taskData.failed_images || 0 }}</span>
          <span>总计: {{ taskData.total_images || 0 }}</span>
        </div>
      </div>

      <!-- 镜像详细状态 -->
      <div class="images-detail">
        <div class="detail-header">
          <span>镜像同步详情</span>
          <div class="filter-controls">
            <el-select v-model="statusFilter" placeholder="状态筛选" size="small" style="width: 120px">
              <el-option label="全部" value="" />
              <el-option label="等待中" value="pending" />
              <el-option label="同步中" value="syncing" />
              <el-option label="成功" value="success" />
              <el-option label="失败" value="failed" />
              <el-option label="重试中" value="retrying" />
            </el-select>
          </div>
        </div>

        <el-table 
          :data="filteredImages" 
          v-loading="loading"
          empty-text="暂无镜像数据"
          max-height="400"
        >
          <el-table-column prop="original_image" label="源镜像" min-width="200">
            <template #default="scope">
              <div class="image-cell">
                <span>{{ scope.row.original_image }}:{{ scope.row.tag }}</span>
                <el-tag size="small" v-if="scope.row.architecture !== 'amd64'">
                  {{ scope.row.architecture }}
                </el-tag>
              </div>
            </template>
          </el-table-column>

          <el-table-column prop="sync_status" label="状态" width="100">
            <template #default="scope">
              <el-tag :type="getImageStatusType(scope.row.sync_status)" size="small">
                {{ getImageStatusText(scope.row.sync_status) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="acr_image" label="目标镜像" min-width="200">
            <template #default="scope">
              <div v-if="scope.row.acr_image" class="target-image">
                <span>{{ scope.row.acr_image }}</span>
                <el-button 
                  size="small" 
                  text 
                  type="primary"
                  @click="copyToClipboard(scope.row.acr_image)"
                >
                  复制
                </el-button>
              </div>
              <span v-else class="text-placeholder">-</span>
            </template>
          </el-table-column>

          <el-table-column prop="duration" label="耗时" width="80">
            <template #default="scope">
              <span v-if="scope.row.duration">{{ scope.row.duration }}s</span>
              <span v-else>-</span>
            </template>
          </el-table-column>

          <el-table-column prop="retry_count" label="重试" width="60">
            <template #default="scope">
              <span v-if="scope.row.retry_count > 0">{{ scope.row.retry_count }}</span>
              <span v-else>-</span>
            </template>
          </el-table-column>

          <el-table-column prop="error_message" label="错误信息" min-width="200">
            <template #default="scope">
              <div v-if="scope.row.error_message" class="error-message">
                <el-tooltip :content="scope.row.error_message" placement="top">
                  <span class="error-text">{{ scope.row.error_message.substring(0, 50) }}...</span>
                </el-tooltip>
              </div>
              <span v-else>-</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 操作按钮 -->
      <div class="task-actions">
        <el-button 
          v-if="taskStatus === 'failed'" 
          type="primary" 
          @click="retryTask"
          :loading="retrying"
        >
          重试失败的镜像
        </el-button>
        <el-button 
          v-if="taskStatus === 'running'" 
          type="warning" 
          @click="pauseTask"
          :loading="pausing"
        >
          暂停任务
        </el-button>
        <el-button 
          v-if="taskStatus === 'paused'" 
          type="primary" 
          @click="resumeTask"
          :loading="resuming"
        >
          恢复任务
        </el-button>
        <el-button 
          v-if="['completed', 'failed'].includes(taskStatus)" 
          @click="clearTask"
        >
          清除任务
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { syncAPI } from '@/api'

// 组件属性
const props = defineProps({
  taskId: {
    type: String,
    required: true
  }
})

// 组件事件
const emit = defineEmits(['clear'])

// 响应式数据
const taskData = ref({})
const loading = ref(false)
const refreshing = ref(false)
const retrying = ref(false)
const pausing = ref(false)
const resuming = ref(false)
const statusFilter = ref('')

// 计算属性
const taskStatus = computed(() => taskData.value.status || 'pending')

const filteredImages = computed(() => {
  const images = taskData.value.images || []
  if (!statusFilter.value) return images
  return images.filter(image => image.sync_status === statusFilter.value)
})

// 状态轮询
let statusPollingTimer = null

// 生命周期
onMounted(() => {
  loadTaskStatus()
  startStatusPolling()
})

onUnmounted(() => {
  stopStatusPolling()
})

// 加载任务状态
const loadTaskStatus = async () => {
  loading.value = true
  try {
    const response = await syncAPI.getBatchSyncStatus(props.taskId)
    taskData.value = response
  } catch (error) {
    ElMessage.error('加载任务状态失败')
  } finally {
    loading.value = false
  }
}

// 刷新状态
const refreshStatus = async () => {
  refreshing.value = true
  try {
    await loadTaskStatus()
    ElMessage.success('状态已刷新')
  } catch (error) {
    ElMessage.error('刷新状态失败')
  } finally {
    refreshing.value = false
  }
}

// 开始状态轮询
const startStatusPolling = () => {
  statusPollingTimer = setInterval(async () => {
    try {
      await loadTaskStatus()
      
      // 如果任务完成，停止轮询
      if (['completed', 'failed'].includes(taskStatus.value)) {
        stopStatusPolling()
      }
    } catch (error) {
      console.error('轮询状态失败:', error)
    }
  }, 5000) // 每5秒轮询一次
}

// 停止状态轮询
const stopStatusPolling = () => {
  if (statusPollingTimer) {
    clearInterval(statusPollingTimer)
    statusPollingTimer = null
  }
}

// 重试任务
const retryTask = async () => {
  retrying.value = true
  try {
    // 这里应该调用重试API
    ElMessage.success('重试请求已提交')
    await loadTaskStatus()
  } catch (error) {
    ElMessage.error('重试任务失败')
  } finally {
    retrying.value = false
  }
}

// 暂停任务
const pauseTask = async () => {
  pausing.value = true
  try {
    // 这里应该调用暂停API
    ElMessage.success('任务已暂停')
    await loadTaskStatus()
  } catch (error) {
    ElMessage.error('暂停任务失败')
  } finally {
    pausing.value = false
  }
}

// 恢复任务
const resumeTask = async () => {
  resuming.value = true
  try {
    // 这里应该调用恢复API
    ElMessage.success('任务已恢复')
    await loadTaskStatus()
  } catch (error) {
    ElMessage.error('恢复任务失败')
  } finally {
    resuming.value = false
  }
}

// 清除任务
const clearTask = () => {
  stopStatusPolling()
  emit('clear')
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

// 工具函数
const getStatusType = (status) => {
  const statusMap = {
    pending: '',
    running: 'warning',
    completed: 'success',
    failed: 'danger',
    paused: 'info'
  }
  return statusMap[status] || ''
}

const getStatusText = (status) => {
  const statusMap = {
    pending: '等待中',
    running: '运行中',
    completed: '已完成',
    failed: '失败',
    paused: '已暂停'
  }
  return statusMap[status] || '未知'
}

const getProgressStatus = () => {
  if (taskStatus.value === 'failed') return 'exception'
  if (taskStatus.value === 'completed') return 'success'
  return null
}

const getImageStatusType = (status) => {
  const statusMap = {
    pending: '',
    syncing: 'warning',
    success: 'success',
    failed: 'danger',
    retrying: 'warning',
    skipped: 'info'
  }
  return statusMap[status] || ''
}

const getImageStatusText = (status) => {
  const statusMap = {
    pending: '等待',
    syncing: '同步中',
    success: '成功',
    failed: '失败',
    retrying: '重试中',
    skipped: '跳过'
  }
  return statusMap[status] || '未知'
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString()
}
</script>

<style scoped>
.batch-sync-status {
  width: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.task-overview {
  margin-bottom: 24px;
}

.progress-section {
  margin-bottom: 24px;
}

.progress-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.progress-text {
  font-weight: 500;
  color: #409eff;
}

.progress-stats {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 12px;
  color: #909399;
}

.images-detail {
  margin-bottom: 24px;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.filter-controls {
  display: flex;
  gap: 12px;
}

.image-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.target-image {
  display: flex;
  align-items: center;
  gap: 8px;
}

.error-message {
  max-width: 200px;
}

.error-text {
  color: #f56c6c;
  cursor: pointer;
}

.text-placeholder {
  color: #c0c4cc;
}

.task-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-start;
}
</style>