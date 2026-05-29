<template>
  <div class="images-manage-view">
    <el-card class="list-card">
      <template #header>
        <div class="card-header">
          <span>镜像管理</span>
          <div class="header-actions">
            <el-select
              v-model="selectedAcrId"
              placeholder="选择 ACR"
              style="width: 300px; margin-right: 16px;"
              @change="handleAcrChange"
            >
              <el-option
                v-for="item in acrList"
                :key="item.id"
                :label="item.namespace"
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
        <el-button type="warning" size="small" @click="handleSyncFromRecords" :loading="syncing">
          从同步记录导入
        </el-button>
      </div>

      <!-- 镜像列表 -->
      <el-table
        :data="repositories"
        v-loading="loading"
        empty-text="暂无镜像数据"
        style="margin-top: 16px;"
      >
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
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { acrRegistryAPI, acrRepositoryAPI } from '@/api'
import { formatTime } from '@/utils/format'
import AddRepositoryDialog from '@/components/AddRepositoryDialog.vue'
import BatchAddRepositoryDialog from '@/components/BatchAddRepositoryDialog.vue'

const router = useRouter()
const route = useRoute()

const acrList = ref([])
const selectedAcrId = ref(null)
const repositories = ref([])
const loading = ref(false)
const syncing = ref(false)

const addDialogVisible = ref(false)
const batchAddDialogVisible = ref(false)

onMounted(() => {
  loadAcrList()
})

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
    console.error('加载 ACR 列表失败:', error)
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
    ElMessage.warning('请先选择 ACR')
    return
  }
  addDialogVisible.value = true
}

const showBatchAddDialog = () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
    return
  }
  batchAddDialogVisible.value = true
}

const goToTags = (row) => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
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

const formatNameList = (names) => (names && names.length ? names.join('、') : '无')

const showSyncImportResult = (data) => {
  const sections = []

  if (data.created_names?.length) {
    sections.push(`成功导入 ${data.created_names.length} 个：\n${formatNameList(data.created_names)}`)
  }
  if (data.missing_in_acr?.length) {
    sections.push(`ACR 中不存在，已跳过 ${data.missing_in_acr.length} 个：\n${formatNameList(data.missing_in_acr)}`)
  }
  if (data.check_failed_names?.length) {
    sections.push(`检查失败，已跳过 ${data.check_failed_names.length} 个：\n${formatNameList(data.check_failed_names)}`)
  }
  if (data.already_exist_names?.length) {
    sections.push(`本地已存在，未重复导入 ${data.already_exist_names.length} 个：\n${formatNameList(data.already_exist_names)}`)
  }

  if (sections.length === 0) {
    ElMessage.info('没有可导入的新镜像')
    return
  }

  const hasIssue = data.missing_in_acr?.length || data.check_failed_names?.length
  ElMessageBox.alert(sections.join('\n\n'), '导入结果', {
    type: hasIssue ? 'warning' : 'success',
    confirmButtonText: '知道了',
  })
}

const handleSyncFromRecords = async () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
    return
  }

  syncing.value = true
  try {
    const response = await acrRepositoryAPI.syncFromRecords(selectedAcrId.value)
    if (response && response.status === 'success') {
      showSyncImportResult(response.data || {})
      loadRepositories()
    }
  } catch (error) {
    console.error('导入失败:', error)
    ElMessage.error('导入失败')
  } finally {
    syncing.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await acrRepositoryAPI.delete(row.id)
    ElMessage.success('删除成功')
    loadRepositories()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}
</script>

<style scoped>
.images-manage-view {
  max-width: var(--max-width);
  margin: 0 auto;
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
</style>
