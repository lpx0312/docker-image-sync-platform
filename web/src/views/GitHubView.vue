<!--
/**
 * GitHub Actions监控页面组件
 * 
 * 功能说明：
 * - 监控GitHub API的使用状态和限制
 * - 展示GitHub Actions工作流运行记录
 * - 提供API限制检查和状态监控
 * - 显示工作流执行历史和详情
 * 
 * 主要功能：
 * - API状态监控：显示剩余请求数、限制和重置时间
 * - 工作流列表：展示GitHub Actions运行历史
 * - 状态检查：实时检查API限制状态
 * - 详情查看：查看具体工作流的执行详情
 * 
 * 监控指标：
 * - 剩余请求数量
 * - API使用率
 * - 限制重置时间
 * - 工作流执行状态
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 */
-->

<template>
  <div class="github-view">
    <!-- GitHub API状态卡片 -->
    <el-card class="status-card">
      <template #header>
        <div class="card-header">
          <span>GitHub API状态</span>
          <el-button 
            type="primary" 
            :icon="Refresh" 
            @click="checkRateLimit"
            :loading="rateLimitLoading"
            size="small"
          >
            检查限制
          </el-button>
        </div>
      </template>

      <div class="rate-limit-info" v-if="rateLimit">
        <el-row :gutter="16">
          <el-col :span="6">
            <div class="limit-item">
              <div class="limit-number">{{ rateLimit.core?.remaining || 0 }}</div>
              <div class="limit-label">剩余请求</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="limit-item">
              <div class="limit-number">{{ rateLimit.core?.limit || 0 }}</div>
              <div class="limit-label">总请求限制</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="limit-item">
              <div class="limit-number">{{ getResetTime() }}</div>
              <div class="limit-label">重置时间</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="limit-item">
              <div class="limit-number" :class="getRateLimitStatus()">
                {{ getRateLimitPercentage() }}%
              </div>
              <div class="limit-label">使用率</div>
            </div>
          </el-col>
        </el-row>
      </div>
    </el-card>

    <!-- GitHub Actions运行列表 -->
    <el-card class="runs-card">
      <template #header>
        <div class="card-header">
          <span>GitHub Actions运行记录</span>
          <div class="header-actions">
            <el-button 
              type="primary" 
              :icon="Refresh" 
              @click="refreshRuns"
              :loading="runsLoading"
              size="small"
            >
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <!-- 筛选器 -->
      <div class="filter-section">
        <el-row :gutter="16">
          <el-col :span="6">
            <el-select
              v-model="statusFilter"
              placeholder="筛选状态"
              clearable
              @change="handleStatusFilter"
            >
              <el-option label="排队中" value="queued" />
              <el-option label="进行中" value="in_progress" />
              <el-option label="已完成" value="completed" />
            </el-select>
          </el-col>
          <el-col :span="6">
            <el-select
              v-model="conclusionFilter"
              placeholder="筛选结果"
              clearable
              @change="handleConclusionFilter"
            >
              <el-option label="成功" value="success" />
              <el-option label="失败" value="failure" />
              <el-option label="取消" value="cancelled" />
              <el-option label="跳过" value="skipped" />
            </el-select>
          </el-col>
          <el-col :span="4">
            <el-button @click="clearFilters">清除筛选</el-button>
          </el-col>
        </el-row>
      </div>

      <!-- 运行列表 -->
      <el-table 
        :data="filteredRuns" 
        v-loading="runsLoading"
        empty-text="暂无运行记录"
      >
        <el-table-column prop="id" label="运行ID" width="120">
          <template #default="{ row }">
            <el-link :href="row.html_url" target="_blank" type="primary">
              #{{ row.id }}
            </el-link>
          </template>
        </el-table-column>
        
        <el-table-column prop="head_sha" label="提交SHA" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ row.head_sha.substring(0, 8) }}</el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="conclusion" label="结果" width="100">
          <template #default="{ row }">
            <el-tag 
              v-if="row.conclusion" 
              :type="getConclusionType(row.conclusion)"
            >
              {{ getConclusionText(row.conclusion) }}
            </el-tag>
            <span v-else>-</span>
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
        
        <el-table-column label="持续时间" width="120">
          <template #default="{ row }">
            {{ getDuration(row) }}
          </template>
        </el-table-column>
        
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button 
                type="text" 
                size="small"
                @click="viewRunDetails(row)"
              >
                详情
              </el-button>
              
              <el-button 
                type="text" 
                size="small"
                @click="openGitHubRun(row.html_url)"
              >
                查看
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="totalRuns"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- 运行详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="GitHub Actions运行详情"
      width="700px"
    >
      <el-descriptions v-if="selectedRun" :column="2" border>
        <el-descriptions-item label="运行ID">
          {{ selectedRun.id }}
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(selectedRun.status)">
            {{ getStatusText(selectedRun.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="结果">
          <el-tag 
            v-if="selectedRun.conclusion" 
            :type="getConclusionType(selectedRun.conclusion)"
          >
            {{ getConclusionText(selectedRun.conclusion) }}
          </el-tag>
          <span v-else>-</span>
        </el-descriptions-item>
        <el-descriptions-item label="提交SHA">
          <el-tag>{{ selectedRun.head_sha }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">
          {{ formatTime(selectedRun.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="更新时间">
          {{ formatTime(selectedRun.updated_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="持续时间" :span="2">
          {{ getDuration(selectedRun) }}
        </el-descriptions-item>
        <el-descriptions-item label="GitHub链接" :span="2">
          <el-link :href="selectedRun.html_url" target="_blank" type="primary">
            在GitHub中查看
          </el-link>
        </el-descriptions-item>
      </el-descriptions>
      
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="detailDialogVisible = false">关闭</el-button>
          <el-button 
            type="primary" 
            @click="openGitHubRun(selectedRun.html_url)"
          >
            在GitHub中查看
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { githubAPI } from '@/api'
import dayjs from 'dayjs'
import duration from 'dayjs/plugin/duration'

dayjs.extend(duration)

// 响应式数据
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

// 计算属性
const filteredRuns = computed(() => {
  let runs = workflowRuns.value
  
  if (statusFilter.value) {
    runs = runs.filter(run => run.status === statusFilter.value)
  }
  
  if (conclusionFilter.value) {
    runs = runs.filter(run => run.conclusion === conclusionFilter.value)
  }
  
  return runs
})

// 检查API速率限制
const checkRateLimit = async () => {
  rateLimitLoading.value = true
  try {
    const response = await githubAPI.getRateLimit()
    rateLimit.value = response.resources
    ElMessage.success('API限制信息已更新')
  } catch (error) {
    ElMessage.error('获取API限制信息失败')
  } finally {
    rateLimitLoading.value = false
  }
}

// 加载GitHub Actions运行记录
const loadWorkflowRuns = async () => {
  runsLoading.value = true
  try {
    const response = await githubAPI.getWorkflowRuns({
      page: currentPage.value,
      per_page: pageSize.value
    })
    workflowRuns.value = response.workflow_runs || []
    totalRuns.value = response.total_count || 0
  } catch (error) {
    ElMessage.error('获取GitHub Actions运行记录失败')
  } finally {
    runsLoading.value = false
  }
}

// 刷新运行记录
const refreshRuns = async () => {
  await loadWorkflowRuns()
  ElMessage.success('运行记录已刷新')
}

// 筛选处理
const handleStatusFilter = () => {
  // 筛选逻辑在计算属性中处理
}

const handleConclusionFilter = () => {
  // 筛选逻辑在计算属性中处理
}

const clearFilters = () => {
  statusFilter.value = ''
  conclusionFilter.value = ''
}

// 分页处理
const handleSizeChange = (size) => {
  pageSize.value = size
  loadWorkflowRuns()
}

const handleCurrentChange = (page) => {
  currentPage.value = page
  loadWorkflowRuns()
}

// 查看运行详情
const viewRunDetails = async (run) => {
  try {
    const response = await githubAPI.getWorkflowRunDetail(run.id)
    selectedRun.value = response
    detailDialogVisible.value = true
  } catch (error) {
    ElMessage.error('获取运行详情失败')
  }
}

// 打开GitHub运行页面
const openGitHubRun = (url) => {
  window.open(url, '_blank')
}

// 工具函数
const getStatusType = (status) => {
  const statusMap = {
    queued: 'info',
    in_progress: 'warning',
    completed: 'success'
  }
  return statusMap[status] || 'info'
}

const getStatusText = (status) => {
  const statusMap = {
    queued: '排队中',
    in_progress: '进行中',
    completed: '已完成'
  }
  return statusMap[status] || status
}

const getConclusionType = (conclusion) => {
  const conclusionMap = {
    success: 'success',
    failure: 'danger',
    cancelled: 'warning',
    skipped: 'info'
  }
  return conclusionMap[conclusion] || 'info'
}

const getConclusionText = (conclusion) => {
  const conclusionMap = {
    success: '成功',
    failure: '失败',
    cancelled: '取消',
    skipped: '跳过'
  }
  return conclusionMap[conclusion] || conclusion
}

const formatTime = (time) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

const getDuration = (run) => {
  if (!run.created_at || !run.updated_at) return '-'
  
  const start = dayjs(run.created_at)
  const end = dayjs(run.updated_at)
  const diff = end.diff(start)
  
  if (diff < 60000) { // 小于1分钟
    return `${Math.floor(diff / 1000)}秒`
  } else if (diff < 3600000) { // 小于1小时
    return `${Math.floor(diff / 60000)}分钟`
  } else {
    const hours = Math.floor(diff / 3600000)
    const minutes = Math.floor((diff % 3600000) / 60000)
    return `${hours}小时${minutes}分钟`
  }
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
  const percentage = getRateLimitPercentage()
  if (percentage >= 80) return 'danger'
  if (percentage >= 60) return 'warning'
  return 'success'
}

// 生命周期
onMounted(() => {
  console.log('GitHubView mounted')
  loadWorkflowRuns().catch(error => {
    console.error('GitHubView loadWorkflowRuns error:', error)
  })
  checkRateLimit().catch(error => {
    console.error('GitHubView checkRateLimit error:', error)
  })
})
</script>

<style scoped>
.github-view {
  max-width: 1400px;
  margin: 0 auto;
}

.status-card,
.runs-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.rate-limit-info {
  margin-top: 16px;
}

.limit-item {
  text-align: center;
  padding: 16px;
  background-color: #f5f7fa;
  border-radius: 4px;
}

.limit-number {
  font-size: 24px;
  font-weight: bold;
  color: #303133;
  margin-bottom: 4px;
}

.limit-number.success {
  color: #67c23a;
}

.limit-number.warning {
  color: #e6a23c;
}

.limit-number.danger {
  color: #f56c6c;
}

.limit-label {
  font-size: 14px;
  color: #909399;
}

.filter-section {
  margin-bottom: 16px;
}

.action-buttons {
  display: flex;
  gap: 8px;
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
</style>