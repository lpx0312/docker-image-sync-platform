<template>
  <div class="acr-tag-list-panel">
    <!-- 搜索区域 -->
    <div class="search-section">
      <el-row :gutter="16">
        <el-col :span="8">
          <el-input
            v-model="searchTag"
            placeholder="搜索 Tag 名称"
            clearable
          />
        </el-col>
        <el-col :span="8">
          <el-input
            v-model="searchDigest"
            placeholder="搜索 SHA256"
            clearable
          />
        </el-col>
        <el-col :span="8">
          <el-select
            v-model="searchArch"
            placeholder="筛选架构"
            clearable
            style="width: 100%;"
          >
            <el-option label="amd64" value="amd64" />
            <el-option label="arm64" value="arm64" />
          </el-select>
        </el-col>
      </el-row>
    </div>

    <!-- Tag 列表 -->
    <el-table
      :data="paginatedTags"
      v-loading="loading"
      empty-text="暂无 Tag 数据"
      style="margin-top: 16px;"
    >
      <el-table-column prop="tag" label="Tag" width="150" />
      <el-table-column label="架构" width="100">
        <template #default="{ row }">
          <div class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <el-tag size="small" :type="getArchTagType(arch)">
                {{ arch }}
              </el-tag>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Digest" min-width="240">
        <template #default="{ row }">
          <div class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <span class="digest-value">{{ row.digests[arch] || '-' }}</span>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="100">
        <template #default="{ row }">
          <div class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <span>{{ formatSize(row.sizes[arch]) }}</span>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="handleCopy(row)">
            复制
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination-section">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="filteredTags.length"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @size-change="handlePageSizeChange"
        @current-change="handlePageChange"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { acrTagAPI } from '@/api'
import { copyToClipboard } from '@/utils/clipboard'

const props = defineProps({
  acrRegistryId: Number,
  repositoryName: String,
  registryUrl: String,
  namespace: String,
})

const loading = ref(false)
const tags = ref([])
const searchTag = ref('')
const searchDigest = ref('')
const searchArch = ref('')
const currentPage = ref(1)
const pageSize = ref(20)

const filteredTags = computed(() => {
  return tags.value.filter(tag => {
    if (searchTag.value && !tag.tag.includes(searchTag.value)) {
      return false
    }
    if (searchArch.value && !tag.architectures.includes(searchArch.value)) {
      return false
    }
    if (searchDigest.value) {
      const hasDigest = Object.values(tag.digests || {}).some(d =>
        d && d.includes(searchDigest.value)
      )
      if (!hasDigest) return false
    }
    return true
  })
})

const paginatedTags = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredTags.value.slice(start, start + pageSize.value)
})

watch([searchTag, searchDigest, searchArch], () => {
  currentPage.value = 1
})

const loadTags = async () => {
  if (!props.acrRegistryId || !props.repositoryName) return

  loading.value = true
  try {
    const response = await acrTagAPI.getTags(props.acrRegistryId, props.repositoryName)
    if (response && response.status === 'success') {
      tags.value = response.data || []
      currentPage.value = 1
    }
  } catch (error) {
    console.error('加载Tag列表失败:', error)
    ElMessage.error('加载Tag列表失败')
  } finally {
    loading.value = false
  }
}

const handlePageSizeChange = () => {
  currentPage.value = 1
}

const handlePageChange = () => {}

const buildImageRef = (tagName) => {
  const registry = (props.registryUrl || '').replace(/^https?:\/\//, '')
  const parts = [registry, props.namespace, props.repositoryName].filter(Boolean)
  return `${parts.join('/')}:${tagName}`
}

const handleCopy = async (tag) => {
  await copyToClipboard(buildImageRef(tag.tag))
}

const getOrderedArchs = (architectures = []) => {
  const order = { amd64: 0, arm64: 1 }
  return [...architectures].sort((a, b) => (order[a] ?? 99) - (order[b] ?? 99))
}

const getArchTagType = (arch) => {
  if (arch === 'amd64') return 'primary'
  if (arch === 'arm64') return 'success'
  return 'info'
}

const formatSize = (bytes) => {
  if (!bytes) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

watch(
  () => [props.acrRegistryId, props.repositoryName],
  () => {
    loadTags()
  }
)

onMounted(() => {
  loadTags()
})

defineExpose({
  loadTags,
  loading,
})
</script>

<style scoped>
.search-section {
  margin-bottom: 16px;
}

.stacked-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.stacked-row {
  min-height: 24px;
  display: flex;
  align-items: center;
}

.digest-value {
  word-break: break-all;
  line-height: 1.4;
}

.pagination-section {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
