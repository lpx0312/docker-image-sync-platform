<!--
/**
 * 镜像列表管理页面组件
 * 
 * 功能说明：
 * - 展示所有镜像的列表和详细信息
 * - 提供镜像的筛选、搜索、排序功能
 * - 支持镜像的删除、重试、检测操作
 * - 显示镜像统计信息和状态分布
 * - 提供批量操作功能
 * 
 * 主要功能：
 * - 镜像列表展示：分页显示镜像信息
 * - 状态筛选：按同步状态筛选镜像
 * - 搜索功能：支持镜像名称搜索
 * - 批量检测：检测多个镜像的存在性
 * - 单个操作：删除、重试、查看详情
 * 
 * 状态管理：
 * - 使用 imageStore 管理镜像数据
 * - 实时更新镜像状态和统计
 * - 支持分页和筛选状态管理
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */
-->

<template>
  <div class="images-view">
    <!-- 统计卡片区域 -->
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

    <!-- 镜像列表 -->
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
              @click="refreshData"
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
          <el-col :span="6">
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

        <el-table-column prop="description" label="同步说明" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.description" class="description-text">
              {{ row.description }}
            </span>
            <span v-else class="no-description">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="architecture" label="架构" width="168" sortable="custom">
          <template #default="{ row }">
            <div class="arch-tags">
              <template v-if="getAcrArchitectures(row).length">
                <el-tag
                  v-for="arch in getAcrArchitectures(row)"
                  :key="arch"
                  size="small"
                  :type="getArchTagType(arch)"
                  class="arch-tag"
                >
                  {{ arch }}
                </el-tag>
              </template>
              <template v-else>
                <el-tag size="small" :type="getArchTagType(row.architecture)">
                  {{ row.architecture || '—' }}
                </el-tag>
              </template>
            </div>
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
              <el-tag :type="getStatusType(row.sync_status)">
                {{ getStatusText(row.sync_status) }}
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
                @click="viewDetails(row)"
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
                @click="retrySync(row)"
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

    <!-- 详情对话框 -->
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
          <el-tag :type="getStatusType(selectedImage.sync_status)">
            {{ getStatusText(selectedImage.sync_status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="架构">
          <div class="arch-tags">
            <template v-if="getAcrArchitectures(selectedImage).length">
              <el-tag
                v-for="arch in getAcrArchitectures(selectedImage)"
                :key="arch"
                size="small"
                :type="getArchTagType(arch)"
                class="arch-tag"
              >
                {{ arch }}
              </el-tag>
            </template>
            <span v-else class="muted-text">{{ selectedImage.architecture || '—' }}</span>
          </div>
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
            @click="retrySync(selectedImage)"
          >
            重试同步
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { 
  Box, 
  CircleCheck, 
  CircleClose, 
  Loading,
  Refresh, 
  Search
} from '@element-plus/icons-vue'
import { useImageStore } from '@/stores/image'
import { configAPI } from '@/api'
import { copyToClipboard } from '@/utils/clipboard'
import { formatTime } from '@/utils/format'
import { getStatusType, getStatusText } from '@/utils/status'

const imageStore = useImageStore()

// 响应式数据
const searchText = ref('')
const statusFilter = ref('')
const architectureFilter = ref('')
const deduplicateEnabled = ref(true) // 去重开关，默认开启
const currentPage = ref(1)
const pageSize = ref(20)
const detailDialogVisible = ref(false)
const selectedImage = ref(null)
const retryingIds = ref([])
const deletingIds = ref([])
const checkingIds = ref([])
const batchChecking = ref(false)
const sortBy = ref('updated_at')
const sortOrder = ref('desc')
const aliyunConfig = ref({
  registry: 'registry.cn-hangzhou.aliyuncs.com',
  namespace: 'docker-sync'
})

// 搜索防抖
let searchTimer = null

// 加载阿里云配置
const loadAliyunConfig = async () => {
  try {
    const response = await configAPI.getAliyunConfig()
    if (response.data) {
      aliyunConfig.value = response.data
    }
  } catch (error) {
    // 配置加载失败时使用默认值即可，不阻断页面
  }
}

// 加载数据
const loadData = async () => {
  await Promise.all([
    imageStore.loadImages(),
    imageStore.loadImageStats(),
    loadAliyunConfig()
  ])
}

// 刷新数据（静默刷新，不弹提示）
const refreshData = async () => {
  await loadData()
}

// 搜索处理
const handleSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    imageStore.updateFilters({ search: searchText.value })
    currentPage.value = 1
    imageStore.updatePagination(1, pageSize.value)
    imageStore.loadImages()
  }, 300)
}

// 状态筛选
const handleStatusFilter = () => {
  imageStore.updateFilters({ status: statusFilter.value })
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  imageStore.loadImages()
}

// 架构筛选
const handleArchitectureFilter = () => {
  imageStore.updateFilters({ architecture: architectureFilter.value })
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  imageStore.loadImages()
}

// 去重处理
const handleDeduplicateChange = () => {
  imageStore.updateFilters({ deduplicate: deduplicateEnabled.value })
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  imageStore.loadImages()
}

// 清除筛选
const clearFilters = () => {
  searchText.value = ''
  statusFilter.value = ''
  architectureFilter.value = ''
  deduplicateEnabled.value = true // 重置去重开关为默认开启
  imageStore.updateFilters({ 
    status: '',
    search: '', 
    architecture: '',
    deduplicate: true
  })
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  imageStore.loadImages()
}

// 排序处理
const handleSortChange = ({ column, prop, order }) => {
  if (prop && order) {
    sortBy.value = prop
    sortOrder.value = order === 'ascending' ? 'asc' : 'desc'
  } else {
    // 清除排序，恢复默认
    sortBy.value = 'created_at'
    sortOrder.value = 'desc'
  }
  
  // 重置到第一页
  currentPage.value = 1
  imageStore.updatePagination(1, pageSize.value)
  
  // 更新排序参数并重新加载数据
  imageStore.updateSorting(sortBy.value, sortOrder.value)
  imageStore.loadImages()
}

// 分页处理
const handleSizeChange = (size) => {
  pageSize.value = size
  imageStore.updatePagination(currentPage.value, size)
  imageStore.loadImages()
}

const handleCurrentChange = (page) => {
  currentPage.value = page
  imageStore.updatePagination(page, pageSize.value)
  imageStore.loadImages()
}

// 查看详情
const viewDetails = async (row) => {
  try {
    selectedImage.value = await imageStore.getImageById(row.id)
    detailDialogVisible.value = true
  } catch (error) {
    ElMessage.error('获取镜像详情失败')
  }
}

// 重试同步
const retrySync = async (row) => {
  retryingIds.value.push(row.id)
  try {
    await imageStore.retrySync(row.id)
    ElMessage.success('重试任务已提交')
    
    // 关闭详情对话框
    if (detailDialogVisible.value) {
      detailDialogVisible.value = false
    }
    
    // 刷新数据
    await loadData()
  } catch (error) {
    ElMessage.error('重试失败')
  } finally {
    retryingIds.value = retryingIds.value.filter(id => id !== row.id)
  }
}

// 删除镜像
const deleteImage = async (row) => {
  deletingIds.value.push(row.id)
  try {
    await imageStore.deleteImage(row.id)
    ElMessage.success('删除成功')
    
    // 如果当前页没有数据了，回到上一页
    if (imageStore.images.length === 0 && currentPage.value > 1) {
      currentPage.value--
      imageStore.updatePagination(currentPage.value, pageSize.value)
      await imageStore.loadImages()
    }
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
    if (result.exists) {
      ElMessage.success(`镜像 ${row.original_image} 检测成功，状态已更新`)
    } else {
      ElMessage.warning(`镜像 ${row.original_image} 不存在，状态已更新为失败`)
    }
    
    // 无论检测结果如何，都重新加载数据以更新状态
    await loadData()
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
    await loadData()
  } catch (error) {
    console.error('批量检测失败:', error)
    ElMessage.error('批量检测失败')
  } finally {
    batchChecking.value = false
  }
}

// 打开GitHub Actions
const openGitHubRun = (url) => {
  window.open(url, '_blank')
}

// 计算行序号
const getRowIndex = (index) => {
  return (currentPage.value - 1) * pageSize.value + index + 1
}

const getAcrArchitectures = (row) => {
  if (!row || !row.acr_architectures) return []
  try {
    const arr = JSON.parse(row.acr_architectures)
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

// Element Plus tag types：与主色（primary 蓝）协调，各架构区分明显
const getArchTagType = (arch) => {
  if (!arch) return 'info'
  const a = String(arch).toLowerCase()
  if (a === 'amd64' || a === 'x86_64') return 'primary'
  if (a === 'arm64' || a === 'aarch64') return 'success'
  if (a === 'arm' || a === 'armv7') return 'warning'
  if (a === 'ppc64le' || a === 's390x' || a === 'riscv64') return 'info'
  return 'info'
}

// 获取目标镜像地址
const getTargetImage = (row) => {
  // 如果 acr_image 有值，直接返回
  if (row.acr_image) {
    return row.acr_image
  }

  // 否则生成默认的 ACR 地址，使用动态配置
  if (row.original_image && row.tag) {
    return `${aliyunConfig.value.registry}/${aliyunConfig.value.namespace}/${row.original_image}:${row.tag}`
  }

  return ''
}

// 监听分页变化
watch([currentPage, pageSize], () => {
  imageStore.updatePagination(currentPage.value, pageSize.value)
})

// 状态轮询
let statusPollingTimer = null

const startStatusPolling = () => {
  // 每5秒轮询一次状态
  statusPollingTimer = setInterval(async () => {
    // 只有在有同步中的镜像时才轮询
    const hasSyncingImages = imageStore.images.some(img => 
      img.sync_status === 'pending' || img.sync_status === 'syncing'
    )
    
    if (hasSyncingImages) {
      console.log('检测到同步中的镜像，自动刷新状态')
      try {
        await Promise.all([
          imageStore.loadImageStats(),
          imageStore.loadImages()
        ])
      } catch (error) {
        console.error('轮询更新失败:', error)
      }
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
  console.log('ImagesView mounted')
  // 同步UI状态与store状态
  statusFilter.value = imageStore.filters.status
  searchText.value = imageStore.filters.search
  architectureFilter.value = imageStore.filters.architecture
  deduplicateEnabled.value = imageStore.filters.deduplicate
  
  loadData().catch(error => {
    console.error('ImagesView loadData error:', error)
  })
  
  // 开始状态轮询
  startStatusPolling()
})

onUnmounted(() => {
  stopStatusPolling()
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
})
</script>

<style scoped>
.images-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.arch-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.arch-tag {
  margin: 0 !important;
}

.muted-text {
  color: var(--el-text-color-secondary);
}

.stat-card {
  position: relative;
  overflow: hidden;
}

.stat-content {
  position: relative;
  z-index: 2;
}

.stat-number {
  font-size: var(--stat-number-size);
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: 4px;
  line-height: 1;
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
  font-size: var(--stat-label-size);
  color: var(--color-text-muted);
  font-weight: 500;
}

.stat-icon {
  position: absolute;
  right: 16px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 40px;
  opacity: 0.1;
  z-index: 1;
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
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filter-section {
  margin-bottom: 16px;
}

.image-info {
  display: flex;
  flex-direction: column;
}

.image-name {
  font-weight: 500;
  margin-bottom: 2px;
}

.image-tag {
  font-size: 12px;
  color: #909399;
}

.action-buttons {
  display: flex;
  gap: 4px;
  align-items: center;
  white-space: nowrap;
}

.danger-button {
  color: #f56c6c !important;
}

.pagination-wrapper {
  margin-top: 16px;
  display: flex;
  justify-content: center;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 4px;
}

.loading-icon {
  animation: spin 1s linear infinite;
  color: #409eff;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.description-text {
  color: #606266;
  font-size: 13px;
  line-height: 1.4;
  word-break: break-all;
}

.no-description {
  color: #c0c4cc;
  font-style: italic;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
  }

  .filter-section .el-row {
    flex-wrap: wrap;
  }

  .filter-section .el-col {
    flex: 0 0 50%;
    max-width: 50%;
    margin-bottom: 8px;
  }
}
</style>