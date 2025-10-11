<template>
  <div class="sync-view">
    <!-- 统计卡片 -->
    <div class="stats-grid">
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-number">{{ imageStore.imageStats.total }}</div>
          <div class="stat-label">总镜像数</div>
        </div>
        <el-icon class="stat-icon total"><Box /></el-icon>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-number success">{{ imageStore.imageStats.success }}</div>
          <div class="stat-label">同步成功</div>
        </div>
        <el-icon class="stat-icon success"><CircleCheck /></el-icon>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-number warning">{{ imageStore.imageStats.syncing }}</div>
          <div class="stat-label">同步中</div>
        </div>
        <el-icon class="stat-icon warning"><Loading /></el-icon>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-number danger">{{ imageStore.imageStats.failed }}</div>
          <div class="stat-label">同步失败</div>
        </div>
        <el-icon class="stat-icon danger"><CircleClose /></el-icon>
      </el-card>
    </div>

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
      </el-tabs>
    </el-card>

    <!-- 镜像列表 -->
    <div class="images-section">
      <el-card class="list-card">
        <template #header>
          <div class="card-header">
            <span>镜像列表</span>
            <div class="header-actions">
              <el-button 
                type="success" 
                @click="batchCheckImages"
                :loading="batchChecking"
                size="small"
              >
                批量检测
              </el-button>
              <el-button 
                type="primary" 
                :icon="Refresh" 
                @click="refreshImageData"
                :loading="imageStore.loading"
                size="small"
              >
                刷新
              </el-button>
            </div>
          </div>
        </template>

        <!-- 搜索和筛选 -->
        <div class="filter-section">
          <el-row :gutter="16">
            <el-col :span="8">
              <el-input
                v-model="searchText"
                placeholder="搜索镜像名称"
                :prefix-icon="Search"
                clearable
                @input="handleSearch"
              />
            </el-col>
            <el-col :span="4">
              <el-select
                v-model="statusFilter"
                placeholder="筛选状态"
                clearable
                @change="handleStatusFilter"
              >
                <el-option label="等待中" value="pending" />
                <el-option label="同步中" value="syncing" />
                <el-option label="成功" value="success" />
                <el-option label="失败" value="failed" />
              </el-select>
            </el-col>
            <el-col :span="4">
              <el-select
                v-model="architectureFilter"
                placeholder="筛选架构"
                clearable
                @change="handleArchitectureFilter"
              >
                <el-option label="amd64" value="amd64" />
                <el-option label="arm64" value="arm64" />
              </el-select>
            </el-col>
            <el-col :span="4">
              <el-checkbox
                v-model="deduplicateEnabled"
                @change="handleDeduplicateChange"
              >
                去重显示
              </el-checkbox>
            </el-col>
            <el-col :span="4">
              <el-button @click="clearFilters">清除筛选</el-button>
            </el-col>
          </el-row>
        </div>

        <!-- 表格 -->
        <el-table 
          :data="imageStore.images" 
          v-loading="imageStore.loading"
          empty-text="暂无镜像记录"
          @sort-change="handleSortChange"
          :default-sort="{ prop: 'created_at', order: 'descending' }"
        >
          <el-table-column type="index" label="序号" width="80" :index="getRowIndex" />
          <el-table-column prop="original_image" label="源镜像" min-width="200" sortable="custom">
            <template #default="{ row }">
              <div class="image-info">
                <div class="image-name">{{ row.original_image }}:{{ row.tag }}</div>
              </div>
            </template>
          </el-table-column>
          
          <el-table-column prop="acr_image" label="目标镜像" min-width="200">
            <template #default="{ row }">
              <div class="image-name">{{ getTargetImage(row) }}</div>
            </template>
          </el-table-column>
          
          <el-table-column prop="architecture" label="架构" width="80" sortable="custom">
            <template #default="{ row }">
              <el-tag :type="row.architecture === 'arm64' ? 'warning' : 'info'" size="small">
                {{ row.architecture || 'amd64' }}
              </el-tag>
            </template>
          </el-table-column>
          
          <el-table-column prop="sync_status" label="状态" width="100" sortable="custom">
            <template #default="{ row }">
              <div class="status-cell">
                <el-icon 
                  v-if="row.sync_status === 'syncing' || row.sync_status === 'pending'" 
                  class="loading-icon"
                >
                  <Loading />
                </el-icon>
                <el-tag :type="getImageStatusType(row.sync_status)">
                  {{ getImageStatusText(row.sync_status) }}
                </el-tag>
              </div>
            </template>
          </el-table-column>
          
          <el-table-column prop="created_at" label="创建时间" width="180" sortable="custom">
            <template #default="{ row }">
              {{ formatTime(row.created_at) }}
            </template>
          </el-table-column>
          
          <el-table-column prop="updated_at" label="更新时间" width="180" sortable="custom">
            <template #default="{ row }">
              {{ formatTime(row.updated_at) }}
            </template>
          </el-table-column>
          
          <el-table-column label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <div class="action-buttons">
                <el-button 
                  v-if="getTargetImage(row) && row.sync_status === 'success'" 
                  type="text" 
                  size="small"
                  @click="copyToClipboard(getTargetImage(row))"
                >
                  复制
                </el-button>
                
                <el-button 
                  type="text" 
                  size="small"
                  @click="viewImageDetails(row)"
                >
                  详情
                </el-button>
                
                <el-button 
                  type="text" 
                  size="small"
                  @click="checkImageExists(row)"
                  :loading="checkingIds.includes(row.id)"
                >
                  检测
                </el-button>
                
                <el-button 
                  v-if="row.status === 'failed'" 
                  type="text" 
                  size="small"
                  @click="retryImageSync(row)"
                  :loading="retryingIds.includes(row.id)"
                >
                  重试
                </el-button>
                
                <el-button 
                  v-if="row.github_run_url" 
                  type="text" 
                  size="small"
                  @click="openGitHubRun(row.github_run_url)"
                >
                  GitHub
                </el-button>
                
                <el-popconfirm
                  title="确定要删除这条记录吗？"
                  @confirm="deleteImage(row)"
                >
                  <template #reference>
                    <el-button 
                      type="text" 
                      size="small"
                      class="danger-button"
                      :loading="deletingIds.includes(row.id)"
                    >
                      删除
                    </el-button>
                  </template>
                </el-popconfirm>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div class="pagination-wrapper">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="imageStore.pagination.total"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </el-card>
    </div>

    <!-- 镜像详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="镜像详情"
      width="600px"
    >
      <el-descriptions v-if="selectedImage" :column="1" border>
        <el-descriptions-item label="ID">
          {{ selectedImage.id }}
        </el-descriptions-item>
        <el-descriptions-item label="源镜像">
          {{ selectedImage.original_image }}:{{ selectedImage.tag }}
        </el-descriptions-item>
        <el-descriptions-item label="目标镜像">
          {{ getTargetImage(selectedImage) }}
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getImageStatusType(selectedImage.sync_status)">
            {{ getImageStatusText(selectedImage.sync_status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="任务ID" v-if="selectedImage.task_id">
          {{ selectedImage.task_id }}
        </el-descriptions-item>
        <el-descriptions-item label="GitHub Run ID" v-if="selectedImage.github_run_id">
          {{ selectedImage.github_run_id }}
        </el-descriptions-item>
        <el-descriptions-item label="GitHub URL" v-if="selectedImage.github_run_url">
          <el-link :href="selectedImage.github_run_url" target="_blank" type="primary">
            查看GitHub Actions
          </el-link>
        </el-descriptions-item>
        <el-descriptions-item label="同步说明" v-if="selectedImage.description">
          {{ selectedImage.description }}
        </el-descriptions-item>
        <el-descriptions-item label="错误信息" v-if="selectedImage.error_message && selectedImage.sync_status === 'failed'">
          <el-alert :title="selectedImage.error_message" type="error" :closable="false" />
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">
          {{ formatTime(selectedImage.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="更新时间">
          {{ formatTime(selectedImage.updated_at) }}
        </el-descriptions-item>
      </el-descriptions>
      
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="detailDialogVisible = false">关闭</el-button>
          <el-button 
            v-if="selectedImage && selectedImage.sync_status === 'failed'" 
            type="primary" 
            @click="retryImageSync(selectedImage)"
          >
            重试同步
          </el-button>
        </div>
      </template>
    </el-dialog>

  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { 
  Refresh, 
  Clock, 
  Check, 
  Close, 
  Warning,
  Link,
  DocumentCopy,
  Loading,
  Box,
  CircleCheck,
  CircleClose,
  Search
} from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'
import { useImageStore } from '@/stores/image'
import SingleSyncForm from '@/components/SingleSyncForm.vue'
import BatchSyncForm from '@/components/BatchSyncForm.vue'
import dayjs from 'dayjs'

// 路由
const router = useRouter()

// 状态管理
const syncStore = useSyncStore()
const imageStore = useImageStore()

// 响应式数据
const refreshing = ref(false)
const activeTab = ref('single')

// 镜像列表相关变量
const searchText = ref('')
const statusFilter = ref('')
const architectureFilter = ref('')
const deduplicateEnabled = ref(true) // 去重开关，默认开启
const currentPage = ref(1)
const pageSize = ref(20)
const retryingIds = ref([])
const deletingIds = ref([])
const checkingIds = ref([])
const batchChecking = ref(false)
const detailDialogVisible = ref(false)
const selectedImage = ref(null)

// 处理单个同步成功
const handleSingleSyncSuccess = async (task) => {
  // 等待一小段时间确保数据已经写入数据库
  await new Promise(resolve => setTimeout(resolve, 500))
  
  // 刷新镜像数据和统计
  await imageStore.loadImageStats()
  await refreshImageData()
}

// 处理批量同步成功
const handleBatchSyncSuccess = async (task) => {
  // 等待一小段时间确保数据已经写入数据库
  await new Promise(resolve => setTimeout(resolve, 500))
  
  // 刷新镜像数据和统计
  await imageStore.loadImageStats()
  await refreshImageData()
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

// 查看同步详情
const viewSyncDetails = (row) => {
  // 跳转到镜像列表页面并高亮显示该记录
  router.push({
    path: '/images',
    query: {
      search: row.source_image,
      highlight: row.id
    }
  })
}

const openGitHubRun = (url) => {
  window.open(url, '_blank')
}

// 镜像相关方法
const refreshImageData = async () => {
  // 先同步本地状态到store
  imageStore.updateFilters({
    search: searchText.value,
    status: statusFilter.value,
    architecture: architectureFilter.value,
    deduplicate: deduplicateEnabled.value
  })
  
  // 更新分页
  imageStore.updatePagination(currentPage.value, pageSize.value)
  
  // 加载镜像数据
  await imageStore.loadImages()
}

const handleSearch = () => {
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  refreshImageData()
}

const handleStatusFilter = () => {
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  refreshImageData()
}

const handleArchitectureFilter = () => {
  currentPage.value = 1
  refreshImageData()
}

// 去重处理
const handleDeduplicateChange = () => {
  imageStore.updateFilters({ deduplicate: deduplicateEnabled.value })
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  imageStore.loadImages()
}

const clearFilters = () => {
  searchText.value = ''
  statusFilter.value = ''
  architectureFilter.value = ''
  deduplicateEnabled.value = true // 重置去重开关为默认开启
  imageStore.updateFilters({ 
    search: '', 
    status: '',
    architecture: '',
    deduplicate: true
  })
  currentPage.value = 1
  refreshImageData()
}

const handleSortChange = ({ prop, order }) => {
  const sortBy = prop
  const sortOrder = order === 'ascending' ? 'asc' : 'desc'
  
  imageStore.loadImages({
    page: currentPage.value,
    page_size: pageSize.value,
    search: searchText.value,
    status: statusFilter.value,
    architecture: architectureFilter.value,
    sort_by: sortBy,
    sort_order: sortOrder
  })
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
  refreshImageData()
}

const handleCurrentChange = (page) => {
  currentPage.value = page
  refreshImageData()
}

const getTargetImage = (row) => {
  if (row.acr_image) {
    // acr_image已经包含完整的镜像地址和标签，不需要再添加tag
    return row.acr_image
  }
  return ''
}

const getImageStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    syncing: 'warning',
    success: 'success',
    failed: 'danger'
  }
  return statusMap[status] || 'info'
}

const getImageStatusText = (status) => {
  const statusMap = {
    pending: '等待中',
    syncing: '同步中',
    success: '成功',
    failed: '失败'
  }
  return statusMap[status] || '未知'
}

// 计算行序号
const getRowIndex = (index) => {
  return (currentPage.value - 1) * pageSize.value + index + 1
}

const viewImageDetails = (row) => {
  selectedImage.value = row
  detailDialogVisible.value = true
}

const retryImageSync = async (row) => {
  if (retryingIds.value.includes(row.id)) return
  
  retryingIds.value.push(row.id)
  try {
    await imageStore.retrySync(row.id)
    ElMessage.success('重试请求已提交')
    refreshImageData()
  } catch (error) {
    ElMessage.error('重试失败')
  } finally {
    retryingIds.value = retryingIds.value.filter(id => id !== row.id)
  }
}

const deleteImage = async (row) => {
  if (deletingIds.value.includes(row.id)) return
  
  deletingIds.value.push(row.id)
  try {
    await imageStore.deleteImage(row.id)
    ElMessage.success('删除成功')
    refreshImageData()
  } catch (error) {
    ElMessage.error('删除失败')
  } finally {
    deletingIds.value = deletingIds.value.filter(id => id !== row.id)
  }
}

// 检测单个镜像是否存在
const checkImageExists = async (row) => {
  checkingIds.value.push(row.id)
  try {
    const result = await imageStore.checkImageExists(row.id)
    const targetImage = getTargetImage(row)
    const imageDisplayName = `${row.architecture}:${targetImage}`
    
    if (result.exists) {
      ElMessage.success(`镜像 ${imageDisplayName} 检测成功，状态已更新`)
    } else {
      ElMessage.warning(`镜像 ${imageDisplayName} 不存在，状态已更新为失败`)
    }
    
    // 无论检测结果如何，都重新加载数据以更新状态
    await refreshImageData()
  } catch (error) {
    console.error('检测镜像失败:', error)
    ElMessage.error('检测镜像失败')
  } finally {
    checkingIds.value = checkingIds.value.filter(id => id !== row.id)
  }
}

// 批量检测镜像
const batchCheckImages = async () => {
  if (imageStore.images.length === 0) {
    ElMessage.warning('没有可检测的镜像')
    return
  }
  
  batchChecking.value = true
  try {
    const imageIds = imageStore.images.map(img => img.id)
    const result = await imageStore.batchCheckImages(imageIds)
    
    ElMessage.success(`批量检测完成：${result.success_count}个镜像存在，${result.failed_count}个镜像不存在`)
    
    // 重新加载数据
    await refreshImageData()
  } catch (error) {
    console.error('批量检测失败:', error)
    ElMessage.error('批量检测失败')
  } finally {
    batchChecking.value = false
  }
}

// 状态轮询
let statusPollingTimer = null

const startStatusPolling = () => {
  // 每5秒轮询一次状态
  statusPollingTimer = setInterval(async () => {
    // 检查是否有当前任务正在进行
    if (syncStore.currentTask && 
        (syncStore.currentTask.status === 'pending' || syncStore.currentTask.status === 'running')) {
      console.log('检测到进行中的同步任务，更新任务状态')
      try {
        // 根据任务类型调用不同的状态更新API
        if (syncStore.currentTask.total_images > 1) {
          // 批量同步任务
          await syncStore.getBatchSyncStatus(syncStore.currentTask.task_id)
        } else {
          // 单个同步任务
          await syncStore.getSyncStatus(syncStore.currentTask.task_id)
        }
      } catch (error) {
        console.error('更新任务状态失败:', error)
      }
    }
    
    // 检查镜像列表中是否有同步中的镜像
    const hasSyncingImages = imageStore.images.some(img => 
      img.sync_status === 'pending' || img.sync_status === 'syncing'
    )
    
    if (hasSyncingImages) {
      console.log('检测到同步中的镜像，自动刷新镜像列表')
      imageStore.loadImageStats()
      refreshImageData()
    }
  }, 5000)
}

const stopStatusPolling = () => {
  if (statusPollingTimer) {
    clearInterval(statusPollingTimer)
    statusPollingTimer = null
  }
}

// 生命周期
onMounted(() => {
  // 同步store状态到本地变量
  searchText.value = imageStore.filters.search
  statusFilter.value = imageStore.filters.status
  architectureFilter.value = imageStore.filters.architecture
  deduplicateEnabled.value = imageStore.filters.deduplicate
  currentPage.value = imageStore.pagination.page
  pageSize.value = imageStore.pagination.pageSize
  
  // 加载镜像数据
  imageStore.loadImageStats()
  refreshImageData()
  
  // 开始状态轮询
  startStatusPolling()
})

onUnmounted(() => {
  // 清理定时器
  stopStatusPolling()
})
</script>

<style scoped>
.sync-view {
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 20px;
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
  flex-direction: column;
  gap: 4px;
}

.source-image {
  font-weight: 500;
  color: #303133;
  word-break: break-all;
}

.target-image {
  margin-top: 4px;
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

/* 镜像部分样式 */
.images-section {
  margin-top: 20px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.stat-card {
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.stat-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.stat-card .el-card__body {
  padding: 20px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stat-content {
  flex: 1;
}

.stat-number {
  font-size: 28px;
  font-weight: bold;
  line-height: 1;
  margin-bottom: 8px;
}

.stat-number.success {
  color: #67c23a;
}

.stat-number.warning {
  color: #e6a23c;
}

.stat-number.danger {
  color: #f56c6c;
}

.stat-label {
  font-size: 14px;
  color: #909399;
  margin: 0;
}

.stat-icon {
  font-size: 32px;
  opacity: 0.8;
}

.stat-icon.total {
  color: #409eff;
}

.stat-icon.success {
  color: #67c23a;
}

.stat-icon.warning {
  color: #e6a23c;
}

.stat-icon.danger {
  color: #f56c6c;
}

.list-card {
  margin-top: 20px;
}

.filter-section {
  margin-bottom: 20px;
}

.action-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.action-buttons .el-button {
  margin: 0;
}

.danger-button {
  color: #f56c6c !important;
}

.danger-button:hover {
  color: #f78989 !important;
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: center;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.loading-icon {
  animation: spin 1s linear infinite;
  color: #409eff;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.dialog-footer {
  text-align: right;
}

.dialog-footer .el-button {
  margin-left: 8px;
}
</style>