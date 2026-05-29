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

    <el-alert
      v-if="allDetailsLoading"
      type="info"
      :closable="false"
      show-icon
      class="search-progress-alert"
      :title="`正在加载详情以支持全量${searchDigest ? ' SHA256' : ''}${searchDigest && searchArch ? ' /' : ''}${searchArch ? ' 架构' : ''}检索（${allDetailsProgress.loaded}/${allDetailsProgress.total}）`"
    />

    <!-- Tag 列表 -->
    <el-table
      :data="paginatedTags"
      v-loading="loading || allDetailsLoading"
      empty-text="暂无 Tag 数据"
      style="margin-top: 16px;"
    >
      <el-table-column prop="tag" label="Tag" width="150" />
      <el-table-column label="架构" width="100">
        <template #default="{ row }">
          <div v-if="isDetailLoading(row.tag)" class="detail-loading">
            <el-skeleton :rows="1" animated />
          </div>
          <div v-else class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <el-tag size="small" :type="getArchTagType(arch)">
                {{ arch }}
              </el-tag>
            </div>
            <span v-if="!row.architectures?.length">-</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Digest" min-width="240">
        <template #default="{ row }">
          <div v-if="isDetailLoading(row.tag)" class="detail-loading">
            <el-skeleton :rows="1" animated />
          </div>
          <div v-else class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <span class="digest-value">{{ row.digests?.[arch] || '-' }}</span>
            </div>
            <span v-if="!row.architectures?.length">-</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="100">
        <template #default="{ row }">
          <div v-if="isDetailLoading(row.tag)" class="detail-loading">
            <el-skeleton :rows="1" animated />
          </div>
          <div v-else class="stacked-cell">
            <div
              v-for="arch in getOrderedArchs(row.architectures)"
              :key="arch"
              class="stacked-row"
            >
              <span>{{ formatSize(row.sizes?.[arch]) }}</span>
            </div>
            <span v-if="!row.architectures?.length">-</span>
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
        :total="filteredTagNames.length"
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

const DETAILS_BATCH_SIZE = 50
const SEARCH_DEBOUNCE_MS = 400

const props = defineProps({
  acrRegistryId: Number,
  repositoryName: String,
  registryUrl: String,
  namespace: String,
})

const loading = ref(false)
const detailsLoading = ref(false)
const allDetailsLoading = ref(false)
const allDetailsProgress = ref({ loaded: 0, total: 0 })
const tagNames = ref([])
const detailCache = ref(new Map())
const loadingTags = ref(new Set())
const searchTag = ref('')
const searchDigest = ref('')
const searchArch = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
let searchDebounceTimer = null
let fullDetailsRequestId = 0

const needsFullDetailsSearch = computed(() => !!(searchDigest.value || searchArch.value))

const emptyDetail = (tag) => ({
  tag,
  architectures: [],
  digests: {},
  sizes: {},
  pushed_at: {},
})

const matchesDigest = (detail, query) => {
  if (!query) return true
  if (!detail) return false
  return Object.values(detail.digests || {}).some(d => d && d.includes(query))
}

const matchesArch = (detail, arch) => {
  if (!arch) return true
  if (!detail) return false
  return detail.architectures?.includes(arch)
}

const filteredTagNames = computed(() => {
  return tagNames.value.filter(tag => {
    if (searchTag.value && !tag.includes(searchTag.value)) {
      return false
    }

    if (!needsFullDetailsSearch.value) {
      return true
    }

    const detail = detailCache.value.get(tag)
    if (!detail) {
      return false
    }

    if (!matchesArch(detail, searchArch.value)) {
      return false
    }
    if (!matchesDigest(detail, searchDigest.value)) {
      return false
    }

    return true
  })
})

const paginatedTags = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredTagNames.value.slice(start, start + pageSize.value).map(tag => {
    return detailCache.value.get(tag) || emptyDetail(tag)
  })
})

const isDetailLoading = (tag) => loadingTags.value.has(tag)

const getCandidateTagsForFullSearch = () => {
  if (!searchTag.value) {
    return [...tagNames.value]
  }
  return tagNames.value.filter(tag => tag.includes(searchTag.value))
}

const updateAllDetailsProgress = () => {
  const candidates = getCandidateTagsForFullSearch()
  const loaded = candidates.filter(tag => detailCache.value.has(tag)).length
  allDetailsProgress.value = {
    loaded,
    total: candidates.length || tagNames.value.length,
  }
}

const clearDetailCache = () => {
  detailCache.value = new Map()
  loadingTags.value = new Set()
  allDetailsLoading.value = false
  allDetailsProgress.value = { loaded: 0, total: 0 }
  fullDetailsRequestId += 1
}

const markTagsLoading = (tags, loading) => {
  const next = new Set(loadingTags.value)
  tags.forEach(tag => {
    if (loading) {
      next.add(tag)
    } else {
      next.delete(tag)
    }
  })
  loadingTags.value = next
}

const loadPageDetails = async (tags) => {
  const pending = tags.filter(tag => !detailCache.value.has(tag) && !loadingTags.value.has(tag))
  if (pending.length === 0) return

  markTagsLoading(pending, true)
  detailsLoading.value = true

  try {
    const response = await acrTagAPI.getTagDetailsBatch(
      props.acrRegistryId,
      props.repositoryName,
      pending
    )
    if (response && response.status === 'success') {
      const nextCache = new Map(detailCache.value)
      for (const detail of response.data || []) {
        nextCache.set(detail.tag, detail)
      }
      for (const tag of pending) {
        if (!nextCache.has(tag)) {
          nextCache.set(tag, emptyDetail(tag))
        }
      }
      detailCache.value = nextCache
      updateAllDetailsProgress()
    }
  } catch (error) {
    console.error('加载Tag详情失败:', error)
    ElMessage.error('加载Tag详情失败')
  } finally {
    markTagsLoading(pending, false)
    detailsLoading.value = false
  }
}

const loadAllDetailsInBatches = async () => {
  const requestId = ++fullDetailsRequestId
  const candidates = getCandidateTagsForFullSearch()
  const uncached = candidates.filter(tag => !detailCache.value.has(tag))

  if (uncached.length === 0) {
    allDetailsLoading.value = false
    updateAllDetailsProgress()
    return
  }

  allDetailsLoading.value = true
  updateAllDetailsProgress()

  try {
    for (let i = 0; i < uncached.length; i += DETAILS_BATCH_SIZE) {
      if (requestId !== fullDetailsRequestId) {
        return
      }

      const batch = uncached.slice(i, i + DETAILS_BATCH_SIZE)
      await loadPageDetails(batch)
    }
  } catch (error) {
    console.error('全量加载Tag详情失败:', error)
    ElMessage.error('全量检索失败，请稍后重试')
  } finally {
    if (requestId === fullDetailsRequestId) {
      allDetailsLoading.value = false
      updateAllDetailsProgress()
    }
  }
}

const scheduleFullDetailsSearch = () => {
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer)
  }

  searchDebounceTimer = setTimeout(async () => {
    currentPage.value = 1

    if (!needsFullDetailsSearch.value) {
      fullDetailsRequestId += 1
      allDetailsLoading.value = false
      const start = (currentPage.value - 1) * pageSize.value
      const pageTags = tagNames.value
        .filter(tag => !searchTag.value || tag.includes(searchTag.value))
        .slice(start, start + pageSize.value)
      await loadPageDetails(pageTags)
      return
    }

    await loadAllDetailsInBatches()
  }, SEARCH_DEBOUNCE_MS)
}

watch(searchTag, () => {
  currentPage.value = 1
  if (needsFullDetailsSearch.value) {
    scheduleFullDetailsSearch()
    return
  }

  const start = (currentPage.value - 1) * pageSize.value
  const pageTags = tagNames.value
    .filter(tag => !searchTag.value || tag.includes(searchTag.value))
    .slice(start, start + pageSize.value)
  loadPageDetails(pageTags)
})

watch([searchDigest, searchArch], () => {
  scheduleFullDetailsSearch()
})

const loadTags = async () => {
  if (!props.acrRegistryId || !props.repositoryName) return

  loading.value = true
  clearDetailCache()
  try {
    const response = await acrTagAPI.getTags(props.acrRegistryId, props.repositoryName)
    if (response && response.status === 'success') {
      tagNames.value = response.data?.tags || []
      currentPage.value = 1
    }
  } catch (error) {
    console.error('加载Tag列表失败:', error)
    ElMessage.error('加载Tag列表失败')
  } finally {
    loading.value = false
  }

  const start = (currentPage.value - 1) * pageSize.value
  const pageTags = filteredTagNames.value.slice(start, start + pageSize.value)
  await loadPageDetails(pageTags)
}

const handlePageSizeChange = () => {
  currentPage.value = 1
}

const handlePageChange = async () => {
  const start = (currentPage.value - 1) * pageSize.value
  const pageTags = filteredTagNames.value.slice(start, start + pageSize.value)
  await loadPageDetails(pageTags)
}

watch(pageSize, async () => {
  const start = (currentPage.value - 1) * pageSize.value
  const pageTags = filteredTagNames.value.slice(start, start + pageSize.value)
  await loadPageDetails(pageTags)
})

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
  detailsLoading,
})
</script>

<style scoped>
.search-section {
  margin-bottom: 16px;
}

.search-progress-alert {
  margin-bottom: 12px;
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

.detail-loading {
  min-width: 60px;
}

.pagination-section {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
