<template>
  <div class="github-view">
    <!-- API 状态卡片 -->
    <div class="section-card">
      <div class="section-header">
        <h3 class="section-title">GitHub API 状态</h3>
        <el-button
          type="primary"
          :icon="Refresh"
          @click="checkRateLimit"
          :loading="rateLimitLoading"
          size="small"
          round
        >
          检查限制
        </el-button>
      </div>

      <div class="rate-grid" v-if="rateLimit">
        <div class="rate-item">
          <span class="rate-value">{{ rateLimit.core?.remaining || 0 }}</span>
          <span class="rate-label">剩余请求</span>
        </div>
        <div class="rate-item">
          <span class="rate-value">{{ rateLimit.core?.limit || 0 }}</span>
          <span class="rate-label">总请求限制</span>
        </div>
        <div class="rate-item">
          <span class="rate-value">{{ getResetTime() }}</span>
          <span class="rate-label">重置时间</span>
        </div>
        <div class="rate-item">
          <span class="rate-value" :class="getRateLimitStatus()">
            {{ getRateLimitPercentage() }}%
          </span>
          <span class="rate-label">使用率</span>
        </div>
      </div>
      <div v-else class="rate-empty">
        <span class="text-muted">点击「检查限制」获取 API 状态</span>
      </div>
    </div>

    <!-- 运行记录 -->
    <div class="section-card">
      <div class="section-header">
        <h3 class="section-title">GitHub Actions 运行记录</h3>
        <el-button
          type="primary"
          :icon="Refresh"
          @click="refreshRuns"
          :loading="runsLoading"
          size="small"
          round
        >
          刷新
        </el-button>
      </div>

      <!-- 筛选 -->
      <div class="filter-bar">
        <el-select
          v-model="statusFilter"
          placeholder="筛选状态"
          clearable
          @change="handleStatusFilter"
          class="filter-select"
        >
          <el-option label="排队中" value="queued" />
          <el-option label="进行中" value="in_progress" />
          <el-option label="已完成" value="completed" />
        </el-select>
        <el-select
          v-model="conclusionFilter"
          placeholder="筛选结果"
          clearable
          @change="handleConclusionFilter"
          class="filter-select"
        >
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failure" />
          <el-option label="取消" value="cancelled" />
          <el-option label="跳过" value="skipped" />
        </el-select>
        <el-button text @click="clearFilters" class="clear-btn">清除筛选</el-button>
      </div>

      <el-table
        :data="filteredRuns"
        v-loading="runsLoading"
        empty-text="暂无运行记录"
        stripe
      >
        <el-table-column prop="id" label="运行 ID" width="120">
          <template #default="{ row }">
            <el-link :href="row.html_url" target="_blank" type="primary">
              #{{ row.id }}
            </el-link>
          </template>
        </el-table-column>

        <el-table-column prop="head_sha" label="提交 SHA" width="120">
          <template #default="{ row }">
            <span class="cell-mono sha-tag">{{ row.head_sha.substring(0, 8) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small" round>
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="conclusion" label="结果" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.conclusion" :type="getConclusionType(row.conclusion)" size="small" round>
              {{ getConclusionText(row.conclusion) }}
            </el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="创建时间" width="170">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="持续时间" width="120">
          <template #default="{ row }">
            <span class="text-secondary">{{ getDuration(row) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button link type="primary" size="small" @click="viewRunDetails(row)">
                详情
              </el-button>
              <el-button link type="primary" size="small" @click="openGitHubRun(row.html_url)">
                查看
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-bar" v-if="!conclusionFilter">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="displayTotal"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
          background
        />
      </div>
      <div v-else class="pagination-bar">
        <span class="text-muted" style="font-size: 13px;">
          当前页筛选结果：{{ filteredRuns.length }} 条记录
        </span>
      </div>
    </div>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="GitHub Actions 运行详情" width="700px" destroy-on-close>
      <el-descriptions v-if="selectedRun" :column="2" border>
        <el-descriptions-item label="运行 ID">{{ selectedRun.id }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(selectedRun.status)" round>
            {{ getStatusText(selectedRun.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="结果">
          <el-tag v-if="selectedRun.conclusion" :type="getConclusionType(selectedRun.conclusion)" round>
            {{ getConclusionText(selectedRun.conclusion) }}
          </el-tag>
          <span v-else class="text-muted">-</span>
        </el-descriptions-item>
        <el-descriptions-item label="提交 SHA">
          <span class="cell-mono">{{ selectedRun.head_sha }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(selectedRun.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatTime(selectedRun.updated_at) }}</el-descriptions-item>
        <el-descriptions-item label="持续时间" :span="2">{{ getDuration(selectedRun) }}</el-descriptions-item>
        <el-descriptions-item label="GitHub 链接" :span="2">
          <el-link :href="selectedRun.html_url" target="_blank" type="primary">
            在 GitHub 中查看
          </el-link>
        </el-descriptions-item>
      </el-descriptions>

      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
        <el-button type="primary" @click="openGitHubRun(selectedRun?.html_url)">
          在 GitHub 中查看
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { githubAPI } from '@/api'
import { formatTime } from '@/utils/format'
import { getGitHubStatusType, getGitHubStatusText, getGitHubConclusionType, getGitHubConclusionText } from '@/utils/status'
import dayjs from 'dayjs'
import duration from 'dayjs/plugin/duration'

dayjs.extend(duration)

const rateLimit = ref(null)
const rateLimitLoading = ref(false)
const workflowRuns = ref([])
const runsLoading = ref(false)
const statusFilter = ref('')
const conclusionFilter = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const totalRuns = ref(0)
const detailDialogVisible = ref(false)
const selectedRun = ref(null)

const filteredRuns = computed(() => {
  let runs = workflowRuns.value
  if (conclusionFilter.value) runs = runs.filter(r => r.conclusion === conclusionFilter.value)
  return runs
})

const displayTotal = computed(() => {
  if (conclusionFilter.value) {
    return filteredRuns.value.length
  }
  return totalRuns.value
})

const checkRateLimit = async () => {
  rateLimitLoading.value = true
  try {
    const response = await githubAPI.getRateLimit()
    rateLimit.value = response.resources
    ElMessage.success('API 限制信息已更新')
  } catch {
    ElMessage.error('获取 API 限制信息失败')
  } finally {
    rateLimitLoading.value = false
  }
}

const loadWorkflowRuns = async () => {
  runsLoading.value = true
  try {
    const params = { page: currentPage.value, per_page: pageSize.value }
    if (statusFilter.value) {
      params.status = statusFilter.value
    }
    const response = await githubAPI.getWorkflowRuns(params)
    workflowRuns.value = response.workflow_runs || []
    totalRuns.value = response.total_count || 0
  } catch {
    ElMessage.error('获取 GitHub Actions 运行记录失败')
  } finally {
    runsLoading.value = false
  }
}

const refreshRuns = async () => {
  await loadWorkflowRuns()
}

const handleStatusFilter = () => { currentPage.value = 1; loadWorkflowRuns() }
const handleConclusionFilter = () => {}
const clearFilters = () => {
  statusFilter.value = ''
  conclusionFilter.value = ''
  currentPage.value = 1
  loadWorkflowRuns()
}

const handleSizeChange = (size) => { pageSize.value = size; loadWorkflowRuns() }
const handleCurrentChange = (page) => { currentPage.value = page; loadWorkflowRuns() }

const viewRunDetails = async (run) => {
  try {
    const response = await githubAPI.getWorkflowRunDetail(run.id)
    selectedRun.value = response
    detailDialogVisible.value = true
  } catch {
    ElMessage.error('获取运行详情失败')
  }
}

const openGitHubRun = (url) => { if (url) window.open(url, '_blank') }

const getStatusType = getGitHubStatusType
const getStatusText = getGitHubStatusText
const getConclusionType = getGitHubConclusionType
const getConclusionText = getGitHubConclusionText

const getDuration = (run) => {
  if (!run.created_at || !run.updated_at) return '-'
  const diff = dayjs(run.updated_at).diff(dayjs(run.created_at))
  if (diff < 60000) return `${Math.floor(diff / 1000)}秒`
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟`
  const h = Math.floor(diff / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  return `${h}小时${m}分钟`
}

const getResetTime = () => {
  if (!rateLimit.value?.core?.reset) return '-'
  return dayjs.unix(rateLimit.value.core.reset).format('HH:mm:ss')
}

const getRateLimitPercentage = () => {
  if (!rateLimit.value?.core) return 0
  const { limit, remaining } = rateLimit.value.core
  return Math.round(((limit - remaining) / limit) * 100)
}

const getRateLimitStatus = () => {
  const p = getRateLimitPercentage()
  if (p >= 80) return 'danger'
  if (p >= 60) return 'warning'
  return 'success'
}

onMounted(() => {
  loadWorkflowRuns().catch(() => {})
  checkRateLimit().catch(() => {})
})
</script>

<style scoped>
.github-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

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

/* ── Rate Limit Grid ── */
.rate-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-md);
}

.rate-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--space-md);
  background: var(--color-bg-muted);
  border-radius: var(--radius-md);
  gap: 4px;
}

.rate-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.rate-value.success { color: var(--color-success); }
.rate-value.warning { color: var(--color-warning); }
.rate-value.danger { color: var(--color-danger); }

.rate-label {
  font-size: 13px;
  color: var(--color-text-muted);
  font-weight: 500;
}

.rate-empty {
  padding: var(--space-xl);
  text-align: center;
}

/* ── Filters ── */
.filter-bar {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  margin-bottom: var(--space-md);
  flex-wrap: wrap;
}

.filter-select { width: 140px; }

.clear-btn {
  color: var(--color-text-muted) !important;
  font-size: 13px;
}

/* ── Table ── */
.cell-mono {
  font-family: var(--font-mono);
  font-size: 12.5px;
}

.sha-tag {
  display: inline-block;
  padding: 2px 8px;
  background: var(--color-bg-muted);
  border-radius: var(--radius-sm);
  color: var(--color-text-secondary);
}

.text-muted { color: var(--color-text-muted); }
.text-secondary { color: var(--color-text-secondary); font-size: 13px; }

.action-buttons {
  display: flex;
  gap: var(--space-xs);
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
@media (max-width: 768px) {
  .rate-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
