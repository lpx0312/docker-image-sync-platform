<template>
  <el-dialog
    v-model="visible"
    :title="`${repositoryName} - Tag 列表`"
    width="900px"
    @close="handleClose"
  >
    <!-- 搜索区域 -->
    <div class="search-section">
      <el-row :gutter="16">
        <el-col :span="8">
          <el-input
            v-model="searchTag"
            placeholder="搜索 Tag 名称"
            clearable
            @input="handleSearch"
          />
        </el-col>
        <el-col :span="8">
          <el-input
            v-model="searchDigest"
            placeholder="搜索 SHA256"
            clearable
            @input="handleSearch"
          />
        </el-col>
        <el-col :span="8">
          <el-select
            v-model="searchArch"
            placeholder="筛选架构"
            clearable
            @change="handleSearch"
          >
            <el-option label="amd64" value="amd64" />
            <el-option label="arm64" value="arm64" />
          </el-select>
        </el-col>
      </el-row>
    </div>

    <!-- Tag 列表 -->
    <el-table
      :data="filteredTags"
      v-loading="loading"
      empty-text="暂无 Tag 数据"
      style="margin-top: 16px;"
    >
      <el-table-column prop="tag" label="Tag" width="150" />
      <el-table-column label="架构" width="120">
        <template #default="{ row }">
          <div class="arch-tags">
            <el-tag
              v-for="arch in row.architectures"
              :key="arch"
              size="small"
              :type="getArchTagType(arch)"
            >
              {{ arch }}
            </el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Digest" min-width="200">
        <template #default="{ row }">
          <div v-for="arch in row.architectures" :key="arch" class="digest-item">
            <el-text type="info" size="small">{{ arch }}:</el-text>
            <el-text size="small" class="digest-value">{{ row.digests[arch] || '-' }}</el-text>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="120">
        <template #default="{ row }">
          <div v-for="arch in row.architectures" :key="arch" class="size-item">
            <el-text type="info" size="small">{{ arch }}:</el-text>
            <el-text size="small">{{ formatSize(row.sizes[arch]) }}</el-text>
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

    <template #footer>
      <el-button @click="handleClose">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { acrTagAPI } from '@/api'
import { copyToClipboard } from '@/utils/clipboard'

const props = defineProps({
  modelValue: Boolean,
  acrRegistryId: Number,
  repositoryName: String,
})

const emit = defineEmits(['update:modelValue'])

const visible = ref(false)
const loading = ref(false)
const tags = ref([])
const searchTag = ref('')
const searchDigest = ref('')
const searchArch = ref('')

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.acrRegistryId && props.repositoryName) {
    loadTags()
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

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

const loadTags = async () => {
  loading.value = true
  try {
    const response = await acrTagAPI.getTags(props.acrRegistryId, props.repositoryName)
    if (response && response.status === 'success') {
      const tagNames = response.data || []
      // 获取每个 tag 的详细信息
      const details = []
      for (const tagName of tagNames) {
        try {
          const detailResp = await acrTagAPI.getTagDetail(
            props.acrRegistryId,
            props.repositoryName,
            tagName
          )
          if (detailResp && detailResp.status === 'success') {
            details.push(detailResp.data)
          }
        } catch (e) {
          console.error(`获取 ${tagName} 详情失败:`, e)
          details.push({
            tag: tagName,
            architectures: [],
            digests: {},
            sizes: {},
          })
        }
      }
      tags.value = details
    }
  } catch (error) {
    console.error('加载Tag列表失败:', error)
    ElMessage.error('加载Tag列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  // 搜索是响应式的，无需额外处理
}

const handleCopy = (tag) => {
  const text = `${props.repositoryName}:${tag.tag}`
  copyToClipboard(text)
  ElMessage.success('已复制到剪贴板')
}

const handleClose = () => {
  visible.value = false
  tags.value = []
  searchTag.value = ''
  searchDigest.value = ''
  searchArch.value = ''
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
</script>

<style scoped>
.search-section {
  margin-bottom: 16px;
}

.arch-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.digest-item,
.size-item {
  display: flex;
  gap: 4px;
  margin-bottom: 2px;
}

.digest-value {
  word-break: break-all;
}
</style>
