<!--
  单个镜像同步表单组件
  
  功能说明：
  - 提供单个Docker镜像的同步配置界面
  - 支持源镜像地址输入和验证
  - 支持 ACR 仓库归属提示与自动预选
  - 支持同步说明描述
  - 集成表单验证和提交处理
-->
<template>
  <div class="single-sync-form">
    <el-form 
      ref="syncFormRef" 
      :model="syncForm" 
      :rules="syncRules" 
      label-width="120px"
    >
      <el-form-item label="目标仓库">
        <el-select
          v-model="selectedAcrId"
          placeholder="选择目标镜像仓库"
          style="width: 100%"
          @change="handleAcrChange"
        >
          <el-option
            v-for="item in acrList"
            :key="item.id"
            :label="getAcrLabel(item)"
            :value="item.id"
            :disabled="isAcrOptionDisabled(item.id)"
          />
        </el-select>
      </el-form-item>

      <div v-if="forceOverrideWarning" class="force-override-warning">
        {{ forceOverrideWarning }}
      </div>

      <el-form-item label="源镜像地址" prop="sourceImage">
        <el-input
          v-model="syncForm.sourceImage"
          placeholder="例如: nginx:latest 或 docker.io/library/nginx:latest"
          clearable
          @input="debouncedSuggestAcr"
        />
        <div class="form-tip">
          支持Docker Hub、Quay.io等公共镜像仓库的镜像
        </div>
      </el-form-item>

      <el-form-item label="同步说明" prop="description">
        <el-input
          v-model="syncForm.description"
          type="textarea"
          :rows="3"
          placeholder="可选：描述此次同步的目的或说明"
          maxlength="500"
          show-word-limit
        />
      </el-form-item>

      <el-form-item>
        <el-button 
          type="primary" 
          @click="submitSync" 
          :loading="syncStore.loading"
        >
          <el-icon><Upload /></el-icon>
          开始同步
        </el-button>
        <el-button @click="resetForm">
          <el-icon><RefreshLeft /></el-icon>
          重置
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Upload, RefreshLeft } from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'
import { acrRegistryAPI, syncAPI } from '@/api'

const emit = defineEmits(['success'])

const syncStore = useSyncStore()
const syncFormRef = ref()

const acrList = ref([])
const quotaMap = ref({})
const selectedAcrId = ref(null)
const currentAffinity = ref(null)
const userChangedAcr = ref(false)

let suggestTimer = null

const syncForm = reactive({
  sourceImage: '',
  description: ''
})

const syncRules = {
  sourceImage: [
    { required: true, message: '请输入源镜像地址', trigger: 'blur' },
    { 
      pattern: /^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$/, 
      message: '请输入有效的镜像地址格式', 
      trigger: 'blur' 
    }
  ]
}

const getAcrNamespace = (acrId) => {
  const acr = acrList.value.find(item => item.id === acrId)
  return acr?.alias || acr?.namespace || `ID:${acrId}`
}

const forceOverrideWarning = computed(() => {
  if (!currentAffinity.value?.has_affinity || !userChangedAcr.value) {
    return ''
  }
  if (selectedAcrId.value === currentAffinity.value.acr_registry_id) {
    return ''
  }
  const forcedNamespace = getAcrNamespace(selectedAcrId.value)
  return `仓库「${currentAffinity.value.repository_name}」已归属镜像仓库「${currentAffinity.value.acr_alias || currentAffinity.value.acr_namespace}」，但强制选择了其他仓库[${forcedNamespace}]`
})

const getAcrLabel = (item) => {
  const typeText = (t) => ({ acr: ' · ACR', swr: ' · SWR', ccr: ' · CCR', harbor: ' · Harbor', generic: ' · Registry' }[t] || '')
  const name = item.alias || item.namespace
  const quota = quotaMap.value[item.id]
  if (quota) {
    const quotaText = quota.repo_quota > 0 ? `${quota.repo_count}/${quota.repo_quota}` : `${quota.repo_count}/不限`
    return `${name} (${quotaText})${typeText(item.registry_type)}`
  }
  return `${name}${typeText(item.registry_type)}`
}

const isAcrOptionDisabled = (acrId) => {
  const quota = quotaMap.value[acrId]
  if (!quota?.is_full) {
    return false
  }
  return !(currentAffinity.value?.has_affinity && currentAffinity.value.acr_registry_id === acrId)
}

const loadAcrData = async () => {
  try {
    const [acrResponse, quotaResponse] = await Promise.all([
      acrRegistryAPI.getAll(),
      acrRegistryAPI.getQuotaSummary(),
    ])

    if (acrResponse?.status === 'success') {
      acrList.value = acrResponse.data || []
      if (!selectedAcrId.value) {
        const defaultAcr = acrList.value.find(item => item.is_default)
        if (defaultAcr) {
          selectedAcrId.value = defaultAcr.id
        }
      }
    }

    if (quotaResponse?.status === 'success') {
      const map = {}
      for (const item of quotaResponse.data || []) {
        map[item.acr_registry_id] = item
      }
      quotaMap.value = map
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
  }
}

const applySuggestion = (data) => {
  currentAffinity.value = data?.affinity || null

  if (!userChangedAcr.value && data?.suggested_acr_id) {
    selectedAcrId.value = data.suggested_acr_id
  }
}

const suggestAcrForInput = async () => {
  const image = syncForm.sourceImage.trim()
  if (!image || !/^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$/.test(image)) {
    currentAffinity.value = null
    return
  }

  try {
    const response = await syncAPI.suggestAcr(image)
    if (response?.status === 'success') {
      const quotaSummary = response.data?.quota_summary || []
      const map = { ...quotaMap.value }
      for (const item of quotaSummary) {
        map[item.acr_registry_id] = item
      }
      quotaMap.value = map
      applySuggestion(response.data)
    }
  } catch (error) {
    console.error('查询 ACR 建议失败:', error)
  }
}

const debouncedSuggestAcr = () => {
  userChangedAcr.value = false
  if (suggestTimer) {
    clearTimeout(suggestTimer)
  }
  suggestTimer = setTimeout(() => {
    suggestAcrForInput()
  }, 500)
}

const handleAcrChange = () => {
  userChangedAcr.value = true
}

onMounted(() => {
  loadAcrData()
})

onUnmounted(() => {
  if (suggestTimer) {
    clearTimeout(suggestTimer)
  }
})

const submitSync = async () => {
  try {
    await syncFormRef.value.validate()

    if (forceOverrideWarning.value) {
      await ElMessageBox.confirm(
        forceOverrideWarning.value + '。确定仍要同步到当前所选 ACR 吗？',
        'ACR 归属风险确认',
        {
          confirmButtonText: '确认风险并继续',
          cancelButtonText: '取消',
          type: 'warning',
        }
      )
    }
    
    const syncData = {
      images: [syncForm.sourceImage],
      description: syncForm.description,
      acr_registry_id: selectedAcrId.value
    }
    
    await syncStore.submitSync(syncData)
    ElMessage.success('同步任务已提交')
    emit('success', syncStore.currentTask)
  } catch (error) {
    if (error === 'cancel' || error?.message === 'cancel') {
      return
    }
    console.error('Submit sync error:', error)
    if (error.errors) {
      return
    }
    ElMessage.error('提交同步任务失败')
  }
}

const resetForm = () => {
  syncFormRef.value.resetFields()
  Object.assign(syncForm, {
    sourceImage: '',
    description: ''
  })
  userChangedAcr.value = false
  currentAffinity.value = null
  const defaultAcr = acrList.value.find(item => item.is_default)
  selectedAcrId.value = defaultAcr ? defaultAcr.id : null
}
</script>

<style scoped>
.single-sync-form {
  width: 100%;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.force-override-warning {
  margin: -8px 0 16px 120px;
  color: #f56c6c;
  font-size: 13px;
  line-height: 1.5;
}
</style>
