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

    <!-- Actions 用量卡片 -->
    <div class="section-card">
      <div class="section-header">
        <h3 class="section-title">GitHub Actions 用量</h3>
        <el-button
          type="primary"
          :icon="Refresh"
          @click="checkActionsUsage"
          :loading="actionsUsageLoading"
          size="small"
          round
        >
          检查用量
        </el-button>
      </div>

      <div class="rate-grid" v-if="actionsUsage">
        <div class="rate-item">
          <span class="rate-value">{{ actionsUsage.total_minutes_used }}</span>
          <span class="rate-label">已用分钟（{{ actionsUsage.period }}·计费加权）</span>
        </div>
        <div class="rate-item">
          <span class="rate-value">{{ actionsUsage.included_minutes || '-' }}</span>
          <span class="rate-label">套餐包含分钟（{{ actionsUsage.plan || '未知套餐' }}）</span>
        </div>
        <div class="rate-item">
          <span class="rate-value">{{ getActionsRemaining() }}</span>
          <span class="rate-label">剩余分钟（估算）</span>
        </div>
        <div class="rate-item">
          <span class="rate-value" :class="getActionsUsageStatus()">
            {{ getActionsUsagePercentage() }}%
          </span>
          <span class="rate-label">使用率（估算）</span>
        </div>
      </div>

      <div class="usage-breakdown" v-if="actionsUsage && skuEntries.length">
        <span class="breakdown-title">按系统：</span>
        <el-tag v-for="[sku, v] in skuEntries" :key="sku" size="small" round type="info">
          {{ sku }}: {{ v }} 分钟
        </el-tag>
      </div>

      <!-- 仓库耗时排行（默认收起，手动展开；展开后默认前 10，其余可继续展开） -->
      <div class="repo-ranking" v-if="actionsUsage && repoEntries.length">
        <button class="ranking-toggle" type="button" @click="rankingExpanded = !rankingExpanded">
          <el-icon class="toggle-icon" :class="{ expanded: rankingExpanded }"><ArrowDown /></el-icon>
          <span>仓库耗时排行</span>
          <span class="toggle-summary">{{ repoEntries.length }} 个仓库 · 合计 {{ totalRepoMinutes }} 分钟</span>
        </button>

        <div v-show="rankingExpanded" class="ranking-panel">
          <div v-for="([repo, v], i) in displayedRepoEntries" :key="repo" class="ranking-row">
            <span class="ranking-index" :class="{ top: i < 3 }">{{ i + 1 }}</span>
            <span class="ranking-name" :title="repo">{{ repo }}</span>
            <div class="ranking-bar">
              <div class="ranking-bar-fill" :style="{ width: barWidth(v) }"></div>
            </div>
            <span class="ranking-minutes">{{ v }} 分钟</span>
            <span class="ranking-percent">{{ percent(v) }}%</span>
          </div>
          <button
            v-if="repoEntries.length > 10"
            class="ranking-more"
            type="button"
            @click="showAllRepos = !showAllRepos"
          >
            {{ showAllRepos ? '收起其余仓库' : `展开其余 ${repoEntries.length - 10} 个仓库` }}
          </button>
        </div>
      </div>

      <div v-if="!actionsUsage" class="rate-empty">
        <span class="text-muted">点击「检查用量」获取本月 Actions 分钟数（公共仓库运行不消耗分钟数）</span>
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
import { Refresh, ArrowDown } from '@element-plus/icons-vue'
import { githubAPI } from '@/api'
import { formatTime } from '@/utils/format'
import { getGitHubStatusType, getGitHubStatusText, getGitHubConclusionType, getGitHubConclusionText } from '@/utils/status'
import dayjs from 'dayjs'
import duration from 'dayjs/plugin/duration'

dayjs.extend(duration)

const rateLimit = ref(null)
const rateLimitLoading = ref(false)
const actionsUsage = ref(null)
const actionsUsageLoading = ref(false)
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

const checkActionsUsage = async () => {
  actionsUsageLoading.value = true
  try {
    actionsUsage.value = await githubAPI.getActionsUsage()
    ElMessage.success('Actions 用量信息已更新')
  } catch {
    // 错误提示由 api 拦截器统一弹出
  } finally {
    actionsUsageLoading.value = false
  }
}

const skuEntries = computed(() => Object.entries(actionsUsage.value?.raw_minutes_by_sku || {}))

// 仓库耗时排行：默认收起；展开后默认显示前 10，其余通过按钮继续展开
const rankingExpanded = ref(false)
const showAllRepos = ref(false)
const repoEntries = computed(() =>
  Object.entries(actionsUsage.value?.minutes_by_repo || {}).sort((a, b) => b[1] - a[1])
)
const displayedRepoEntries = computed(() =>
  showAllRepos.value ? repoEntries.value : repoEntries.value.slice(0, 10)
)
const totalRepoMinutes = computed(() => repoEntries.value.reduce((sum, [, v]) => sum + v, 0))
const barWidth = (v) => (totalRepoMinutes.value ? `${Math.max((v / totalRepoMinutes.value) * 100, 2)}%` : '0%')
const percent = (v) => (totalRepoMinutes.value ? Math.round((v / totalRepoMinutes.value) * 100) : 0)

const getActionsRemaining = () => {
  const u = actionsUsage.value
  if (!u || !u.included_minutes) return '-'
  return Math.max(u.included_minutes - u.total_minutes_used, 0)
}

const getActionsUsagePercentage = () => {
  const u = actionsUsage.value
  if (!u || !u.included_minutes) return 0
  return Math.round((u.total_minutes_used / u.included_minutes) * 100)
}

const getActionsUsageStatus = () => {
  const p = getActionsUsagePercentage()
  if (p >= 80) return 'danger'
  if (p >= 60) return 'warning'
  return 'success'
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
  checkActionsUsage().catch(() => {})
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

/* ── Actions Usage Breakdown ── */
.usage-breakdown {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--space-xs);
  margin-top: var(--space-md);
}

.breakdown-title {
  font-size: 13px;
  color: var(--color-text-muted);
  font-weight: 500;
  margin-right: var(--space-xs);
}

/* ── Repo Ranking ── */
.repo-ranking {
  margin-top: var(--space-md);
}

.ranking-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-xs);
  width: 100%;
  padding: var(--space-sm) var(--space-md);
  background: var(--color-bg-muted);
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
  color: var(--color-text-primary);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.2s;
}

.ranking-toggle:hover {
  background: var(--color-bg-card);
}

.toggle-icon {
  font-size: 12px;
  color: var(--color-text-muted);
  transition: transform 0.2s;
}

.toggle-icon.expanded {
  transform: rotate(180deg);
}

.toggle-summary {
  margin-left: auto;
  color: var(--color-text-muted);
  font-weight: 500;
}

.ranking-panel {
  margin-top: var(--space-sm);
  padding: var(--space-sm) var(--space-md);
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
}

.ranking-row {
  display: grid;
  grid-template-columns: 24px minmax(140px, 220px) 1fr 76px 44px;
  align-items: center;
  gap: var(--space-sm);
  padding: 6px 0;
}

.ranking-index {
  font-size: 13px;
  font-weight: 700;
  color: var(--color-text-muted);
  text-align: center;
}

.ranking-index.top {
  color: var(--color-warning);
}

.ranking-name {
  font-size: 13px;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ranking-bar {
  height: 8px;
  background: var(--color-bg-muted);
  border-radius: 4px;
  overflow: hidden;
}

.ranking-bar-fill {
  height: 100%;
  border-radius: 4px;
  background: var(--el-color-primary);
  opacity: 0.75;
}

.ranking-minutes {
  font-size: 13px;
  color: var(--color-text-secondary);
  text-align: right;
  white-space: nowrap;
}

.ranking-percent {
  font-size: 12.5px;
  color: var(--color-text-muted);
  text-align: right;
}

.ranking-more {
  display: block;
  margin: var(--space-sm) auto 0;
  padding: 2px 12px;
  background: none;
  border: none;
  color: var(--el-color-primary);
  font-size: 13px;
  cursor: pointer;
}

.ranking-more:hover {
  text-decoration: underline;
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

  .ranking-row {
    grid-template-columns: 24px minmax(90px, 1fr) 70px;
  }

  .ranking-row .ranking-bar {
    display: none;
  }

  .ranking-row .ranking-percent {
    display: none;
  }
}
</style>
