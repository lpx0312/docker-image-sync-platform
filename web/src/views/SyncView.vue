<template>
  <div class="sync-view">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div
        v-for="stat in statCards"
        :key="stat.key"
        class="stat-card"
        :class="stat.key"
      >
        <div class="stat-left">
          <span class="stat-value">{{ stat.value }}</span>
          <span class="stat-label">{{ stat.label }}</span>
        </div>
        <div class="stat-icon-wrap" :class="stat.key">
          <component :is="stat.icon" />
        </div>
      </div>
    </div>

    <!-- 同步操作区 -->
    <div class="section-card">
      <div class="section-header">
        <h3 class="section-title">镜像同步</h3>
        <el-button
          type="primary"
          :icon="Refresh"
          @click="refreshStatus"
          :loading="refreshing"
          size="small"
          round
        >
          刷新状态
        </el-button>
      </div>

      <el-tabs v-model="activeTab" class="sync-tabs">
        <el-tab-pane label="单个同步" name="single">
          <SingleSyncForm @success="handleSyncSuccess" />
        </el-tab-pane>
        <el-tab-pane label="批量同步" name="batch">
          <BatchSyncForm @success="handleSyncSuccess" />
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- 镜像列表 -->
    <div class="section-card">
      <div class="section-header">
        <h3 class="section-title">镜像列表</h3>
        <div class="header-actions">
          <el-button
            type="success"
            @click="batchCheckImages"
            :loading="batchChecking"
            size="small"
            round
          >
            批量检测
          </el-button>
          <el-button
            type="primary"
            :icon="Refresh"
            @click="refreshImageData"
            :loading="imageStore.loading"
            size="small"
            round
          >
            刷新
          </el-button>
        </div>
      </div>

      <!-- 筛选区 -->
      <div class="filter-bar">
        <el-input
          v-model="searchText"
          placeholder="搜索镜像名称..."
          :prefix-icon="Search"
          clearable
          @input="handleSearch"
          class="filter-search"
        />
        <el-select
          v-model="statusFilter"
          placeholder="状态"
          clearable
          @change="handleStatusFilter"
          class="filter-select"
        >
          <el-option label="等待中" value="pending" />
          <el-option label="同步中" value="syncing" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-select
          v-model="architectureFilter"
          placeholder="架构"
          clearable
          @change="handleArchitectureFilter"
          class="filter-select-sm"
        >
          <el-option label="amd64" value="amd64" />
          <el-option label="arm64" value="arm64" />
        </el-select>
        <el-checkbox
          v-model="deduplicateEnabled"
          @change="handleDeduplicateChange"
          class="filter-checkbox"
        >
          去重显示
        </el-checkbox>
        <el-button text @click="clearFilters" class="clear-btn">清除筛选</el-button>
      </div>

      <!-- 表格 -->
      <el-table
        :data="imageStore.images"
        v-loading="imageStore.loading"
        empty-text="暂无镜像记录"
        @sort-change="handleSortChange"
        :default-sort="{ prop: 'created_at', order: 'descending' }"
        stripe
        class="image-table"
      >
        <el-table-column type="index" label="#" width="60" :index="getRowIndex" />
        <el-table-column prop="original_image" label="源镜像" min-width="200" sortable="custom">
          <template #default="{ row }">
            <span class="cell-mono">{{ row.original_image }}:{{ row.tag }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="acr_image" label="目标镜像" min-width="200">
          <template #default="{ row }">
            <span class="cell-mono" v-if="getTargetImage(row)">{{ getTargetImage(row) }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="description" label="说明" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.description">{{ row.description }}</span>
            <span v-else class="text-muted">-</span>
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
                  round
                  :type="getArchTagType(arch)"
                  class="arch-tag"
                >
                  {{ arch }}
                </el-tag>
              </template>
              <template v-else>
                <el-tag size="small" round :type="getArchTagType(row.architecture)">
                  {{ row.architecture || '—' }}
                </el-tag>
              </template>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="sync_status" label="状态" width="110" sortable="custom">
          <template #default="{ row }">
            <div class="status-cell">
              <span
                v-if="row.sync_status === 'syncing' || row.sync_status === 'pending'"
                class="status-dot pulsing"
              ></span>
              <el-tag :type="getImageStatusType(row.sync_status)" size="small" round>
                {{ getImageStatusText(row.sync_status) }}
              </el-tag>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="创建时间" width="170" sortable="custom">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button
                v-if="getTargetImage(row) && row.sync_status === 'success'"
                link
                type="primary"
                size="small"
                @click="copyToClipboard(getTargetImage(row))"
              >
                复制
              </el-button>
              <el-button link type="primary" size="small" @click="viewImageDetails(row)">
                详情
              </el-button>
              <el-button
                link
                type="primary"
                size="small"
                @click="checkImageExists(row)"
                :loading="checkingIds.includes(row.id)"
              >
                检测
              </el-button>
              <el-button
                v-if="row.sync_status === 'failed'"
                link
                type="warning"
                size="small"
                @click="retryImageSync(row)"
                :loading="retryingIds.includes(row.id)"
              >
                重试
              </el-button>
              <el-button
                v-if="row.github_run_url"
                link
                type="primary"
                size="small"
                @click="openGitHubRun(row.github_run_url)"
              >
                GitHub
              </el-button>
              <el-popconfirm title="确定要删除这条记录吗？" @confirm="deleteImage(row)">
                <template #reference>
                  <el-button
                    link
                    type="danger"
                    size="small"
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
      <div class="pagination-bar">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="imageStore.pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
          background
        />
      </div>
    </div>

    <!-- 镜像详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="镜像详情" width="600px" destroy-on-close>
      <el-descriptions v-if="selectedImage" :column="1" border>
        <el-descriptions-item label="ID">{{ selectedImage.id }}</el-descriptions-item>
        <el-descriptions-item label="源镜像">
          <span class="cell-mono">{{ selectedImage.original_image }}:{{ selectedImage.tag }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="目标镜像">
          <span class="cell-mono">{{ getTargetImage(selectedImage) }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getImageStatusType(selectedImage.sync_status)" round>
            {{ getImageStatusText(selectedImage.sync_status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="架构">
          <div class="arch-tags">
            <template v-if="getAcrArchitectures(selectedImage).length">
              <el-tag
                v-for="arch in getAcrArchitectures(selectedImage)"
                :key="arch"
                size="small"
                round
                :type="getArchTagType(arch)"
                class="arch-tag"
              >
                {{ arch }}
              </el-tag>
            </template>
            <template v-else>
              <el-tag size="small" round :type="getArchTagType(selectedImage.architecture)">
                {{ selectedImage.architecture || '—' }}
              </el-tag>
            </template>
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
            查看 GitHub Actions
          </el-link>
        </el-descriptions-item>
        <el-descriptions-item label="同步说明" v-if="selectedImage.description">
          {{ selectedImage.description }}
        </el-descriptions-item>
        <el-descriptions-item
          label="错误信息"
          v-if="selectedImage.error_message && selectedImage.sync_status === 'failed'"
        >
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
        <el-button @click="detailDialogVisible = false">关闭</el-button>
        <el-button
          v-if="selectedImage && selectedImage.sync_status === 'failed'"
          type="primary"
          @click="retryImageSync(selectedImage)"
        >
          重试同步
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Refresh,
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
import { copyToClipboard } from '@/utils/clipboard'
import { formatTime } from '@/utils/format'

const syncStore = useSyncStore()
const imageStore = useImageStore()

const refreshing = ref(false)
const activeTab = ref('single')
const searchText = ref('')
const statusFilter = ref('')
const architectureFilter = ref('')
const deduplicateEnabled = ref(true)
const currentPage = ref(1)
const pageSize = ref(20)
const retryingIds = ref([])
const deletingIds = ref([])
const checkingIds = ref([])
const batchChecking = ref(false)
const detailDialogVisible = ref(false)
const selectedImage = ref(null)

const statCards = computed(() => [
  { key: 'total', label: '总镜像数', value: imageStore.imageStats.total, icon: Box },
  { key: 'success', label: '同步成功', value: imageStore.imageStats.success, icon: CircleCheck },
  { key: 'syncing', label: '同步中', value: imageStore.imageStats.syncing, icon: Loading },
  { key: 'failed', label: '同步失败', value: imageStore.imageStats.failed, icon: CircleClose },
])

const refreshStatus = async () => {
  refreshing.value = true
  try {
    await imageStore.loadImageStats()
    await refreshImageData()
  } finally {
    refreshing.value = false
  }
}

const handleSyncSuccess = async () => {
  await new Promise(resolve => setTimeout(resolve, 500))
  await imageStore.loadImageStats()
  await refreshImageData()
}

const refreshImageData = async () => {
  imageStore.updateFilters({
    search: searchText.value,
    status: statusFilter.value,
    architecture: architectureFilter.value,
    deduplicate: deduplicateEnabled.value
  })
  imageStore.updatePagination(currentPage.value, pageSize.value)
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
  deduplicateEnabled.value = true
  imageStore.updateFilters({ search: '', status: '', architecture: '', deduplicate: true })
  currentPage.value = 1
  refreshImageData()
}

const handleSortChange = ({ prop, order }) => {
  imageStore.loadImages({
    page: currentPage.value,
    page_size: pageSize.value,
    search: searchText.value,
    status: statusFilter.value,
    architecture: architectureFilter.value,
    sort_by: prop,
    sort_order: order === 'ascending' ? 'asc' : 'desc'
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

const getTargetImage = (row) => row.acr_image || ''

const getImageStatusType = (status) => {
  const map = { pending: 'info', syncing: 'warning', success: 'success', failed: 'danger' }
  return map[status] || 'info'
}

const getImageStatusText = (status) => {
  const map = { pending: '等待中', syncing: '同步中', success: '成功', failed: '失败' }
  return map[status] || '未知'
}

const getRowIndex = (index) => (currentPage.value - 1) * pageSize.value + index + 1

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
  } catch {
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
  } catch {
    ElMessage.error('删除失败')
  } finally {
    deletingIds.value = deletingIds.value.filter(id => id !== row.id)
  }
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

const getArchTagType = (arch) => {
  if (!arch) return 'info'
  const a = String(arch).toLowerCase()
  if (a === 'amd64' || a === 'x86_64') return 'primary'
  if (a === 'arm64' || a === 'aarch64') return 'success'
  if (a === 'arm' || a === 'armv7') return 'warning'
  if (a === 'ppc64le' || a === 's390x' || a === 'riscv64') return 'info'
  return 'info'
}

const archDisplayLabel = (row) => {
  const list = getAcrArchitectures(row)
  if (list.length) return list.join(', ')
  return row?.architecture || '—'
}

const checkImageExists = async (row) => {
  checkingIds.value.push(row.id)
  try {
    const result = await imageStore.checkImageExists(row.id)
    const targetImage = getTargetImage(row)
    const displayName = `${archDisplayLabel(row)}:${targetImage}`
    if (result.exists) {
      ElMessage.success(`镜像 ${displayName} 检测成功，状态已更新`)
    } else {
      ElMessage.warning(`镜像 ${displayName} 不存在，状态已更新为失败`)
    }
    await refreshImageData()
  } catch {
    ElMessage.error('检测镜像失败')
  } finally {
    checkingIds.value = checkingIds.value.filter(id => id !== row.id)
  }
}

const batchCheckImages = async () => {
  if (imageStore.images.length === 0) {
    ElMessage.warning('没有可检测的镜像')
    return
  }
  batchChecking.value = true
  try {
    const imageIds = imageStore.images.map(img => img.id)
    const result = await imageStore.batchCheckImages(imageIds)
    ElMessage.success(`批量检测完成：${result.success_count}个存在，${result.failed_count}个不存在`)
    await refreshImageData()
  } catch {
    ElMessage.error('批量检测失败')
  } finally {
    batchChecking.value = false
  }
}

const openGitHubRun = (url) => window.open(url, '_blank')

let statusPollingTimer = null

const startStatusPolling = () => {
  statusPollingTimer = setInterval(async () => {
    if (syncStore.currentTask &&
        (syncStore.currentTask.status === 'pending' || syncStore.currentTask.status === 'running')) {
      try {
        await syncStore.getSyncStatus(syncStore.currentTask.task_id)
      } catch { /* polling error ignored */ }
    }

    const hasSyncingImages = imageStore.images.some(img =>
      img.sync_status === 'pending' || img.sync_status === 'syncing'
    )
    if (hasSyncingImages) {
      imageStore.loadImageStats()
      refreshImageData()
    }
  }, 5000)
}

onMounted(() => {
  searchText.value = imageStore.filters.search
  statusFilter.value = imageStore.filters.status
  architectureFilter.value = imageStore.filters.architecture
  deduplicateEnabled.value = imageStore.filters.deduplicate
  currentPage.value = imageStore.pagination.page
  pageSize.value = imageStore.pagination.pageSize
  imageStore.loadImageStats()
  refreshImageData()
  startStatusPolling()
})

onUnmounted(() => {
  if (statusPollingTimer) {
    clearInterval(statusPollingTimer)
    statusPollingTimer = null
  }
})
</script>

<style scoped>
.sync-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

/* ── Stat Cards ── */
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-md);
  margin-bottom: var(--space-lg);
}

.stat-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px var(--space-lg);
  background: var(--color-bg-card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
  box-shadow: var(--shadow-sm);
  transition: all var(--transition-base);
  cursor: default;
}

.stat-card:hover {
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
}

.stat-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-value {
  font-size: var(--stat-number-size);
  font-weight: 700;
  line-height: 1;
  color: var(--color-text-primary);
}

.stat-card.success .stat-value { color: var(--color-success); }
.stat-card.syncing .stat-value { color: var(--color-warning); }
.stat-card.failed .stat-value { color: var(--color-danger); }

.stat-label {
  font-size: var(--stat-label-size);
  color: var(--color-text-muted);
  font-weight: 500;
}

.stat-icon-wrap {
  width: 44px;
  height: 44px;
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
}

.stat-icon-wrap.total { background: var(--color-primary-bg); color: var(--color-primary); }
.stat-icon-wrap.success { background: var(--color-success-bg); color: var(--color-success); }
.stat-icon-wrap.syncing { background: var(--color-warning-bg); color: var(--color-warning); }
.stat-icon-wrap.failed { background: var(--color-danger-bg); color: var(--color-danger); }

/* ── Section Card ── */
.section-card {
  background: var(--color-bg-card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
  box-shadow: var(--shadow-sm);
  padding: var(--space-lg);
  margin-bottom: var(--space-lg);
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-md);
}

.section-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.header-actions {
  display: flex;
  gap: var(--space-sm);
}

/* ── Filters ── */
.filter-bar {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  margin-bottom: var(--space-md);
  flex-wrap: wrap;
}

.filter-search {
  width: 260px;
}

.filter-select {
  width: 130px;
}

.filter-select-sm {
  width: 110px;
}

.filter-checkbox {
  margin-left: var(--space-sm);
}

.clear-btn {
  color: var(--color-text-muted) !important;
  font-size: 13px;
}

/* ── Table ── */
.cell-mono {
  font-family: var(--font-mono);
  font-size: 12.5px;
  word-break: break-all;
  color: var(--color-text-primary);
}

.text-muted {
  color: var(--color-text-muted);
}

.text-secondary {
  color: var(--color-text-secondary);
  font-size: 13px;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-warning-light);
  flex-shrink: 0;
}

.status-dot.pulsing {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.8); }
}

.action-buttons {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
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

/* ── Pagination ── */
.pagination-bar {
  display: flex;
  justify-content: center;
  margin-top: var(--space-lg);
  padding-top: var(--space-md);
  border-top: 1px solid var(--color-border-light);
}

/* ── Responsive ── */
@media (max-width: 1024px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .stats-row {
    grid-template-columns: 1fr;
  }

  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-search,
  .filter-select,
  .filter-select-sm {
    width: 100%;
  }
}
</style>
