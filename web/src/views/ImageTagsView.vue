<template>
  <div class="image-tags-view">
    <el-breadcrumb separator="/" class="page-breadcrumb">
      <el-breadcrumb-item :to="{ path: '/images' }">镜像管理</el-breadcrumb-item>
      <el-breadcrumb-item :to="{ path: '/images', query: { acrId: acrId } }">
        {{ acrNamespace || '-' }}
      </el-breadcrumb-item>
      <el-breadcrumb-item :to="currentTagsRoute">{{ repoName }}</el-breadcrumb-item>
      <el-breadcrumb-item>Tag 列表</el-breadcrumb-item>
    </el-breadcrumb>

    <el-card class="list-card">
      <template #header>
        <div class="card-header">
          <span>{{ repoName }} - Tag 列表</span>
          <el-button
            type="primary"
            :icon="Refresh"
            @click="handleRefresh"
            :loading="refreshing"
            size="small"
          >
            刷新
          </el-button>
        </div>
      </template>

      <AcrTagListPanel
        ref="panelRef"
        :acr-registry-id="acrId"
        :repository-name="repoName"
        :registry-url="acrRegistryUrl"
        :namespace="acrNamespace"
      />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import { acrRegistryAPI } from '@/api'
import AcrTagListPanel from '@/components/AcrTagListPanel.vue'

const router = useRouter()
const route = useRoute()
const panelRef = ref(null)
const acrNamespace = ref('')
const acrRegistryUrl = ref('')
const refreshing = ref(false)

const acrId = computed(() => {
  const id = Number(route.params.acrId)
  return Number.isFinite(id) && id > 0 ? id : null
})

const repoName = computed(() => {
  const name = route.params.repoName
  return typeof name === 'string' ? name : ''
})

const currentTagsRoute = computed(() => ({
  name: 'ImageTags',
  params: {
    acrId: acrId.value,
    repoName: repoName.value,
  },
}))

const loadAcrInfo = async () => {
  if (!acrId.value) return

  try {
    const response = await acrRegistryAPI.getAll()
    if (response && response.status === 'success') {
      const acr = (response.data || []).find(item => item.id === acrId.value)
      acrNamespace.value = acr?.namespace || ''
      acrRegistryUrl.value = acr?.registry_url || ''
    }
  } catch (error) {
    console.error('加载 ACR 信息失败:', error)
  }
}

const handleRefresh = async () => {
  refreshing.value = true
  try {
    await panelRef.value?.loadTags()
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  if (!acrId.value || !repoName.value) {
    router.replace('/images')
    return
  }
  loadAcrInfo()
})
</script>

<style scoped>
.image-tags-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

.page-breadcrumb {
  margin-bottom: var(--space-md);
  font-size: 14px;
}

.page-breadcrumb :deep(.el-breadcrumb__inner.is-link) {
  color: var(--color-text-secondary);
  font-weight: 500;
}

.page-breadcrumb :deep(.el-breadcrumb__inner.is-link:hover) {
  color: var(--color-primary);
}

.page-breadcrumb :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--color-text-primary);
  font-weight: 600;
}

.list-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
