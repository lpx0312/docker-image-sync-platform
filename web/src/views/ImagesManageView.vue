<template>
  <div class="images-manage-view">
    <!-- 镜像仓库配额概览 -->
    <el-card v-if="quotaSummary.length" class="quota-card">
      <template #header>
        <div class="card-header">
          <span>镜像仓库配额</span>
          <el-button type="warning" link size="small" @click="showDuplicateReport">
            检测跨仓库重复
          </el-button>
        </div>
      </template>
      <div class="quota-list">
        <div v-for="item in quotaSummary" :key="item.acr_registry_id" class="quota-item">
          <div class="quota-label">
            <span>
              {{ item.alias || item.namespace }}
              <el-tag :type="item.registry_type === 'swr' ? 'warning' : item.registry_type === 'ccr' ? 'success' : 'primary'" size="small" style="margin-left: 6px;">
                {{ typeLabel(item) }}
              </el-tag>
            </span>
            <span class="quota-count">{{ formatQuotaCount(item) }}</span>
          </div>
          <el-progress
            v-if="item.repo_quota > 0"
            :percentage="getQuotaPercent(item)"
            :status="item.is_full ? 'exception' : (getQuotaPercent(item) >= 80 ? 'warning' : '')"
            :stroke-width="10"
          />
        </div>
      </div>
    </el-card>

    <el-card class="list-card">
      <template #header>
        <div class="card-header">
          <span>镜像管理</span>
          <div class="header-actions">
            <el-select
              v-model="selectedAcrId"
              placeholder="选择镜像仓库"
              style="width: 300px; margin-right: 16px;"
              @change="handleAcrChange"
            >
              <el-option
                v-for="item in acrList"
                :key="item.id"
                :label="getAcrLabel(item)"
                :value="item.id"
              />
            </el-select>
            <el-button
              type="primary"
              :icon="Refresh"
              @click="loadRepositories"
              :loading="loading"
              size="small"
            >
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <!-- 操作按钮 -->
      <div class="action-section">
        <el-button type="primary" size="small" @click="showAddDialog">
          添加镜像
        </el-button>
        <el-button type="success" size="small" @click="showBatchAddDialog">
          批量添加
        </el-button>
        <el-button
          type="success"
          size="small"
          plain
          @click="handleImportFromRegistry"
          :loading="importing"
        >
          从仓库导入
        </el-button>
        <el-button
          type="danger"
          size="small"
          :disabled="selectedRows.length === 0"
          @click="handleBatchDelete"
        >
          批量删除{{ selectedRows.length ? ` (${selectedRows.length})` : '' }}
        </el-button>
        <el-button type="warning" size="small" @click="handleSyncFromRecords" :loading="syncing">
          从同步记录导入
        </el-button>
        <el-button type="info" size="small" @click="handleCleanInvalid" :loading="cleaning">
          清理无效镜像
        </el-button>
      </div>

      <!-- 搜索 -->
      <div class="filter-section">
        <el-input
          v-model="searchText"
          placeholder="搜索镜像名称"
          :prefix-icon="Search"
          clearable
          style="max-width: 320px;"
          @input="handleSearch"
        />
      </div>

      <!-- 镜像列表 -->
      <el-table
        ref="tableRef"
        :data="paginatedRepositories"
        v-loading="loading"
        empty-text="暂无镜像数据"
        style="margin-top: 16px;"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column type="index" label="序号" width="60" />
        <el-table-column prop="repository_name" label="镜像名称" min-width="200">
          <template #default="{ row }">
            <el-button type="primary" link @click="goToTags(row)">
              {{ row.repository_name }}
            </el-button>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.updated_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="goToTags(row)">
              查看 Tag
            </el-button>
            <el-popconfirm
              title="确定要删除这个镜像吗？"
              @confirm="handleDelete(row)"
            >
              <template #reference>
                <el-button type="danger" link size="small">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-section">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredRepositories.length"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @size-change="handlePageSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 添加镜像对话框 -->
    <AddRepositoryDialog
      v-model="addDialogVisible"
      :acr-registry-id="selectedAcrId"
      @success="loadRepositories"
    />

    <!-- 批量添加对话框 -->
    <BatchAddRepositoryDialog
      v-model="batchAddDialogVisible"
      :acr-registry-id="selectedAcrId"
      @success="loadRepositories"
    />

    <!-- 统一结果报告弹窗 -->
    <ResultReportDialog
      v-model="report.visible"
      :title="report.title"
      :tone="report.tone"
      :summary="report.summary"
      :sections="report.sections"
      :confirm-text="report.confirmText"
      :cancel-text="report.cancelText"
      :confirm-type="report.confirmType"
      :empty-text="report.emptyText"
      :width="report.width"
      @confirm="confirmReport"
      @cancel="cancelReport"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import { acrRegistryAPI, acrRepositoryAPI } from '@/api'
import { formatTime } from '@/utils/format'
import AddRepositoryDialog from '@/components/AddRepositoryDialog.vue'
import BatchAddRepositoryDialog from '@/components/BatchAddRepositoryDialog.vue'
import ResultReportDialog from '@/components/ResultReportDialog.vue'
import { useResultReport } from '@/composables/useResultReport'

const router = useRouter()
const route = useRoute()

const acrList = ref([])
const selectedAcrId = ref(null)
const repositories = ref([])
const loading = ref(false)
const syncing = ref(false)
const cleaning = ref(false)
const quotaSummary = ref([])
const quotaMap = ref({})
const selectedRows = ref([])
const tableRef = ref(null)

const addDialogVisible = ref(false)
const batchAddDialogVisible = ref(false)
const importing = ref(false)

const { report, openReport, confirmReport, cancelReport } = useResultReport()

const registryTypeBadge = (t) => ({
  acr: 'ACR', swr: 'SWR', ccr: 'CCR', harbor: 'Harbor', generic: 'Registry',
}[t] || (t ? t.toUpperCase() : 'ACR'))

const registryTagTone = (t) => ({ swr: 'warning', harbor: 'info', generic: 'info' }[t] || 'primary')
const searchText = ref('')
const currentPage = ref(1)
const pageSize = ref(20)

let searchTimer = null

const filteredRepositories = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  if (!keyword) {
    return repositories.value
  }
  return repositories.value.filter(item =>
    item.repository_name.toLowerCase().includes(keyword)
  )
})

const paginatedRepositories = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredRepositories.value.slice(start, start + pageSize.value)
})

const handleSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    currentPage.value = 1
  }, 300)
}

const handlePageSizeChange = () => {
  currentPage.value = 1
}

const handlePageChange = () => {
  // 分页由 computed 自动处理
}

const handleSelectionChange = (rows) => {
  selectedRows.value = rows
}

const clearSelection = () => {
  selectedRows.value = []
  tableRef.value?.clearSelection()
}

onMounted(() => {
  loadAcrList()
  loadQuotaSummary()
})

const typeLabel = (row) => ({
  acr: '阿里 ACR',
  swr: '华为 SWR',
  ccr: '腾讯 CCR',
  harbor: 'Harbor',
  generic: '通用',
}[row.registry_type] || row.registry_type || 'acr')

const typeText = (t) => ({ acr: ' · ACR', swr: ' · SWR', ccr: ' · CCR', harbor: ' · Harbor', generic: ' · Registry' }[t] || '')

const getAcrLabel = (item) => {
  const name = item.alias || item.namespace
  const quota = quotaMap.value[item.id]
  if (quota) {
    return `${name} (${formatQuotaCount(quota)})${typeText(item.registry_type)}`
  }
  return `${name}${typeText(item.registry_type)}`
}

const formatQuotaCount = (item) => {
  if (!item || !item.repo_quota) return `${item?.repo_count ?? 0}/不限`
  return `${item.repo_count}/${item.repo_quota}`
}

const getQuotaPercent = (item) => {
  if (!item.repo_quota) return 0
  return Math.min(100, Math.round((item.repo_count / item.repo_quota) * 100))
}

const loadQuotaSummary = async () => {
  try {
    const response = await acrRegistryAPI.getQuotaSummary()
    if (response?.status === 'success') {
      quotaSummary.value = response.data || []
      const map = {}
      for (const item of quotaSummary.value) {
        map[item.acr_registry_id] = item
      }
      quotaMap.value = map
    }
  } catch (error) {
    console.error('加载仓库配额失败:', error)
  }
}

const showDuplicateReport = async () => {
  try {
    const response = await acrRepositoryAPI.getDuplicates()
    const duplicates = response?.data || []
    if (!duplicates.length) {
      ElMessage.success('未发现跨仓库重复的仓库')
      return
    }

    openReport({
      title: '跨仓库重复仓库',
      tone: 'warning',
      summary: `共 ${duplicates.length} 个仓库名同时存在于多个镜像仓库，同步时注意按别名区分`,
      width: '600px',
      sections: [{
        label: '重复仓库',
        tone: 'warning',
        items: duplicates.map(item => ({
          title: item.repository_name,
          tags: (item.registries?.length ? item.registries : []).map(r => ({
            label: `${r.alias || r.namespace} · ${registryTypeBadge(r.registry_type)}`,
            tone: registryTagTone(r.registry_type),
          })),
        })),
      }],
    })
  } catch (error) {
    console.error('查询重复仓库失败:', error)
    ElMessage.error('查询重复仓库失败')
  }
}

const applyAcrSelection = () => {
  const queryAcrId = Number(route.query.acrId)
  if (queryAcrId && acrList.value.some(item => item.id === queryAcrId)) {
    selectedAcrId.value = queryAcrId
    loadRepositories()
    return true
  }
  return false
}

watch(
  () => route.query.acrId,
  () => {
    if (acrList.value.length > 0) {
      applyAcrSelection()
    }
  }
)

const loadAcrList = async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    if (response && response.status === 'success') {
      acrList.value = response.data || []
      if (!applyAcrSelection()) {
        const defaultAcr = acrList.value.find(item => item.is_default)
        if (defaultAcr) {
          selectedAcrId.value = defaultAcr.id
          loadRepositories()
        }
      }
    }
  } catch (error) {
    console.error('加载镜像仓库列表失败:', error)
  }
}

const handleAcrChange = () => {
  if (selectedAcrId.value) {
    router.replace({ path: '/images', query: { acrId: selectedAcrId.value } })
    loadRepositories()
  }
}

const loadRepositories = async () => {
  if (!selectedAcrId.value) return

  loading.value = true
  try {
    const response = await acrRepositoryAPI.getAll(selectedAcrId.value)
    if (response && response.status === 'success') {
      repositories.value = response.data || []
      currentPage.value = 1
      clearSelection()
    }
  } catch (error) {
    console.error('加载镜像列表失败:', error)
    ElMessage.error('加载镜像列表失败')
  } finally {
    loading.value = false
  }
}

const showAddDialog = () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }
  addDialogVisible.value = true
}

const showBatchAddDialog = () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }
  batchAddDialogVisible.value = true
}

const goToTags = (row) => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }
  router.push({
    name: 'ImageTags',
    params: {
      acrId: selectedAcrId.value,
      repoName: row.repository_name,
    },
  })
}

const showSyncImportResult = (data) => {
  const sections = [
    { label: '成功导入', tone: 'success', items: data.created_names || [] },
    { label: '目标仓库中不存在，已跳过', tone: 'warning', items: data.missing_in_acr || [] },
    { label: '检查失败，已跳过', tone: 'danger', items: data.check_failed_names || [] },
    { label: '本地已存在，未重复导入', tone: 'info', items: data.already_exist_names || [] },
  ].filter(s => s.items.length > 0)

  if (!sections.length) {
    ElMessage.info('没有可导入的新镜像')
    return
  }

  const hasIssue = data.missing_in_acr?.length || data.check_failed_names?.length
  openReport({
    title: '导入结果',
    tone: hasIssue ? 'warning' : 'success',
    sections,
  })
}

const handleSyncFromRecords = async () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }

  syncing.value = true
  try {
    const response = await acrRepositoryAPI.syncFromRecords(selectedAcrId.value)
    if (response && response.status === 'success') {
      showSyncImportResult(response.data || {})
      await Promise.all([loadRepositories(), loadQuotaSummary()])
    }
  } catch (error) {
    console.error('导入失败:', error)
    ElMessage.error('导入失败')
  } finally {
    syncing.value = false
  }
}

const handleImportFromRegistry = async () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }

  importing.value = true
  try {
    const response = await acrRepositoryAPI.importFromRegistry(selectedAcrId.value)
    if (response && response.status === 'success') {
      const data = response.data || {}
      const sections = [
        { label: '成功导入', tone: 'success', items: data.created_names || [] },
        { label: '本地已存在，未重复导入', tone: 'info', items: data.already_exist_names || [] },
      ].filter(s => s.items.length > 0)

      if (!sections.length) {
        ElMessage.info('远程仓库中没有可导入的新镜像')
      } else {
        openReport({
          title: '从仓库导入结果',
          tone: 'success',
          sections,
        })
      }
      await Promise.all([loadRepositories(), loadQuotaSummary()])
    }
  } catch (error) {
    console.error('从仓库导入失败:', error)
    ElMessage.error('从仓库导入失败: ' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    importing.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await acrRepositoryAPI.delete(row.id)
    ElMessage.success('删除成功')
    await Promise.all([loadRepositories(), loadQuotaSummary()])
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

const handleBatchDelete = async () => {
  if (!selectedRows.value.length) {
    ElMessage.warning('请先选择要删除的镜像')
    return
  }

  try {
    const confirmed = await openReport({
      title: '批量删除确认',
      tone: 'warning',
      summary: `确定要删除以下 ${selectedRows.value.length} 个镜像记录吗？此操作不会删除远程仓库中的实际镜像。`,
      cancelText: '取消',
      confirmText: '确定删除',
      confirmType: 'danger',
      sections: [{
        label: '待删除镜像',
        tone: 'warning',
        items: selectedRows.value.map(row => row.repository_name),
      }],
    })
    if (!confirmed) return

    const ids = selectedRows.value.map(row => row.id)
    const response = await acrRepositoryAPI.batchDelete(ids)
    if (response?.status === 'success') {
      ElMessage.success(response.message || '批量删除成功')
      await Promise.all([loadRepositories(), loadQuotaSummary()])
    }
  } catch (error) {
    console.error('批量删除失败:', error)
    ElMessage.error('批量删除失败')
  }
}

const handleCleanInvalid = async () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择镜像仓库')
    return
  }

  const acrLabel = acrList.value.find(item => item.id === selectedAcrId.value)?.alias || acrList.value.find(item => item.id === selectedAcrId.value)?.namespace || '当前仓库'

  try {
    await ElMessageBox.confirm(
      `将检查 ${acrLabel} 下的本地镜像列表，并清理在目标仓库中不存在的无效记录。此操作不会删除远程仓库中的实际镜像。`,
      '清理无效镜像',
      {
        type: 'warning',
        confirmButtonText: '开始清理',
        cancelButtonText: '取消',
      }
    )

    cleaning.value = true
    const response = await acrRepositoryAPI.cleanInvalid(selectedAcrId.value)
    if (response?.status === 'success') {
      const data = response.data || {}
      const sections = [
        { label: '已清理无效记录', tone: 'success', items: data.cleaned_names || [] },
        { label: '检查失败，未清理', tone: 'danger', items: data.check_failed_names || [] },
      ].filter(s => s.items.length > 0)

      if (!sections.length) {
        ElMessage.success('未发现无效镜像，无需清理')
      } else {
        openReport({
          title: '清理结果',
          tone: data.check_failed_names?.length ? 'warning' : 'success',
          sections,
        })
      }
      await Promise.all([loadRepositories(), loadQuotaSummary()])
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('清理无效镜像失败:', error)
      ElMessage.error('清理无效镜像失败')
    }
  } finally {
    cleaning.value = false
  }
}
</script>

<style scoped>
.images-manage-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

.quota-card {
  margin-bottom: 20px;
}

.quota-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.quota-item {
  width: 100%;
}

.quota-label {
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
  font-size: 14px;
}

.quota-count {
  color: #909399;
}

.list-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
}

.action-section {
  display: flex;
  gap: 8px;
}

.filter-section {
  margin-top: 16px;
}

.pagination-section {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
