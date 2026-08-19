<!--
  批量镜像同步表单组件
-->
<template>
  <div class="batch-sync-form">
    <el-form 
      ref="batchFormRef" 
      :model="batchForm" 
      :rules="batchRules" 
      label-width="120px"
    >
      <el-form-item label="目标仓库">
        <el-select
          v-model="selectedAcrId"
          placeholder="选择目标镜像仓库"
          style="width: 100%"
          :disabled="isMultiImage"
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

      <div v-if="isMultiImage" class="multi-acr-hint">
        同步多个镜像时目标仓库将自动选择，无法进行手动调整
      </div>

      <div v-else-if="forceOverrideWarning" class="force-override-warning">
        {{ forceOverrideWarning }}
      </div>

      <el-form-item label="镜像列表" prop="images">
        <div class="image-input-section">
          <el-radio-group v-model="inputMode" class="input-mode-selector">
            <el-radio-button label="manual">手动输入</el-radio-button>
            <el-radio-button label="file">文件导入</el-radio-button>
          </el-radio-group>

          <div v-if="inputMode === 'manual'" class="manual-input">
            <el-input
              v-model="imageInput"
              type="textarea"
              :rows="8"
              :placeholder="getInputPlaceholder()"
              @input="debouncedParseImageInput"
            />
            <div class="input-tips">
              <el-text type="info" size="small">
                支持格式：镜像名:标签，每行一个。例如：nginx:latest
              </el-text>
            </div>
          </div>

          <div v-if="inputMode === 'file'" class="file-input">
            <el-upload
              ref="uploadRef"
              :auto-upload="false"
              :on-change="handleFileChange"
              :before-remove="handleFileRemove"
              accept=".txt,.csv"
              :limit="1"
            >
              <template #trigger>
                <el-button type="primary">
                  <el-icon><Upload /></el-icon>
                  选择文件
                </el-button>
              </template>
              <template #tip>
                <div class="el-upload__tip">
                  支持 .txt 和 .csv 文件，每行一个镜像地址
                </div>
              </template>
            </el-upload>
          </div>

          <div v-if="parsedImages.length > 0" class="image-preview">
            <div class="preview-header">
              <span>镜像预览 ({{ parsedImages.length }} 个镜像)</span>
              <el-button size="small" @click="clearImages">清空</el-button>
            </div>
            <div class="image-list">
              <div
                v-for="(image, index) in parsedImages"
                :key="index"
                class="image-item"
              >
                <div class="image-info">
                  <el-tag class="image-tag" type="info">{{ image }}</el-tag>
                </div>
                <el-button 
                  size="small" 
                  type="danger" 
                  text 
                  @click="removeImage(index)"
                >
                  删除
                </el-button>
              </div>
            </div>
          </div>
        </div>
      </el-form-item>

      <el-form-item label="同步说明" prop="description">
        <el-input
          v-model="batchForm.description"
          type="textarea"
          :rows="3"
          placeholder="可选：描述此次批量同步的目的或说明"
          maxlength="500"
          show-word-limit
        />
        <div class="form-tip">
          描述此次批量同步的目的，将应用到所有镜像任务
        </div>
      </el-form-item>

      <el-form-item>
        <el-button 
          type="primary" 
          @click="submitBatchSync" 
          :loading="loading"
          :disabled="parsedImages.length === 0"
        >
          <el-icon><Upload /></el-icon>
          开始批量同步 ({{ parsedImages.length }} 个镜像)
        </el-button>
        <el-button 
          type="warning" 
          @click="submitMockBatchSync" 
          :loading="mockLoading"
          :disabled="parsedImages.length === 0"
        >
          <el-icon><Upload /></el-icon>
          模拟批量同步 ({{ parsedImages.length }} 个镜像)
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
import { syncAPI, acrRegistryAPI } from '@/api'

const emit = defineEmits(['success'])

const batchFormRef = ref()
const uploadRef = ref()

const inputMode = ref('manual')

const acrList = ref([])
const selectedAcrId = ref(null)
const quotaMap = ref({})
const currentAffinity = ref(null)
const userChangedAcr = ref(false)

const batchForm = reactive({
  description: ''
})

const imageInput = ref('')
const parsedImages = ref([])
const loading = ref(false)
const mockLoading = ref(false)
let parseDebounceTimer = null
let suggestTimer = null

const batchRules = {}

const isSingleImage = computed(() => parsedImages.value.length === 1)
const isMultiImage = computed(() => parsedImages.value.length > 1)

const getAcrNamespace = (acrId) => {
  const acr = acrList.value.find(item => item.id === acrId)
  return acr?.alias || acr?.namespace || `ID:${acrId}`
}

const forceOverrideWarning = computed(() => {
  if (!isSingleImage.value || !currentAffinity.value?.has_affinity || !userChangedAcr.value) {
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
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        selectedAcrId.value = defaultAcr.id
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

const suggestAcrForSingleImage = async () => {
  if (!isSingleImage.value) {
    currentAffinity.value = null
    return
  }

  const image = parsedImages.value[0]
  if (!image || !isValidImageFormat(image)) {
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

const debouncedSuggestAcrForImages = () => {
  userChangedAcr.value = false
  if (suggestTimer) {
    clearTimeout(suggestTimer)
  }
  suggestTimer = setTimeout(() => {
    suggestAcrForSingleImage()
  }, 500)
}

const handleAcrChange = () => {
  if (isSingleImage.value) {
    userChangedAcr.value = true
  }
}

onMounted(async () => {
  await loadAcrData()
})

const getInputPlaceholder = () =>
  `每行一个镜像，格式：镜像名:标签（不要写 --platform，多架构由同步流水线处理）
例如：
nginx:latest
redis:6.2
mysql:8.0`

const validateInput = (inputText) => {
  const lines = inputText.split('\n')
  const errors = []
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    const lineNumber = i + 1
    if (line.includes('--platform')) {
      errors.push(`第${lineNumber}行：请勿使用 --platform，请只填写 镜像:标签`)
      continue
    }
    if (!isValidImageFormat(line)) {
      errors.push(`第${lineNumber}行：格式应为 镜像名:标签`)
    }
  }
  return errors
}

const debouncedParseImageInput = () => {
  if (parseDebounceTimer) {
    clearTimeout(parseDebounceTimer)
  }
  parseDebounceTimer = setTimeout(() => {
    parseImageInput()
  }, 500)
}

const parseImageInput = () => {
  const validationErrors = validateInput(imageInput.value)
  if (validationErrors.length > 0) {
    ElMessage.error({
      message: validationErrors.join('\n'),
      duration: 5000
    })
    return
  }

  const lines = imageInput.value.split('\n')
  const seen = new Set()
  const images = []
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed || !isValidImageFormat(trimmed)) continue
    if (seen.has(trimmed)) continue
    seen.add(trimmed)
    images.push(trimmed)
  }
  parsedImages.value = images
  debouncedSuggestAcrForImages()
}

const isValidImageFormat = (image) => {
  const imageRegex = /^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$/
  return imageRegex.test(image)
}

const handleFileChange = (file) => {
  const reader = new FileReader()
  reader.onload = (e) => {
    const content = e.target.result
    parseFileContent(content, file.name)
  }
  reader.readAsText(file.raw)
}

const parseFileContent = (content, fileName) => {
  const images = []
  
  if (fileName.endsWith('.csv')) {
    const lines = content.split('\n')
    for (const line of lines) {
      const columns = line.split(',')
      for (const column of columns) {
        const trimmed = column.trim().replace(/['"]/g, '')
        if (trimmed && isValidImageFormat(trimmed)) {
          if (!images.includes(trimmed)) {
            images.push(trimmed)
          }
        }
      }
    }
  } else {
    const lines = content.split('\n')
    for (const line of lines) {
      const trimmed = line.trim()
      if (trimmed && isValidImageFormat(trimmed)) {
        if (!images.includes(trimmed)) {
          images.push(trimmed)
        }
      }
    }
  }
  
  parsedImages.value = images
  imageInput.value = images.join('\n')
  
  ElMessage.success(`成功解析 ${images.length} 个镜像`)
  debouncedSuggestAcrForImages()
}

const handleFileRemove = () => {
  parsedImages.value = []
  imageInput.value = ''
  currentAffinity.value = null
}

const rebuildImageInput = () => {
  imageInput.value = parsedImages.value.join('\n')
}

const removeImage = (index) => {
  parsedImages.value.splice(index, 1)
  rebuildImageInput()
  debouncedSuggestAcrForImages()
}

const doClearImages = () => {
  parsedImages.value = []
  imageInput.value = ''
  currentAffinity.value = null
  userChangedAcr.value = false
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
}

const clearImages = async () => {
  if (parsedImages.value.length === 0) {
    doClearImages()
    return
  }
  try {
    await ElMessageBox.confirm(
      `确定要清空全部 ${parsedImages.value.length} 个镜像吗？`,
      '确认清空',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
    doClearImages()
  } catch {
    // 用户取消
  }
}

const buildBatchImageItems = () =>
  parsedImages.value.map((line) => {
    const [imageName, tag] = line.split(':')
    return {
      source_image: imageName,
      target_tag: tag || 'latest',
      architecture: '',
      priority: 1,
      description: batchForm.description
    }
  })

const confirmRiskIfNeeded = async () => {
  if (!isSingleImage.value || !forceOverrideWarning.value) {
    return
  }

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

const submitBatchSync = async () => {
  try {
    await batchFormRef.value.validate()

    if (parsedImages.value.length === 0) {
      ElMessage.warning('请至少添加一个镜像')
      return
    }

    await confirmRiskIfNeeded()

    loading.value = true

    const batchData = {
      images: buildBatchImageItems(),
      max_concurrent: 0,
      auto_retry: true,
      retry_count: 0,
      acr_registry_id: isMultiImage.value ? 0 : selectedAcrId.value
    }

    const response = await syncAPI.submitBatchSync(batchData)
    
    ElMessage.success('批量同步任务已提交')
    emit('success', response)
    resetForm()
    
  } catch (error) {
    if (error === 'cancel' || error?.message === 'cancel') {
      return
    }
    if (error.errors) {
      return
    }
    ElMessage.error('提交批量同步任务失败')
  } finally {
    loading.value = false
  }
}

const submitMockBatchSync = async () => {
  try {
    await batchFormRef.value.validate()

    if (parsedImages.value.length === 0) {
      ElMessage.warning('请至少添加一个镜像')
      return
    }

    await confirmRiskIfNeeded()

    mockLoading.value = true

    const batchData = {
      images: buildBatchImageItems(),
      max_concurrent: 0,
      auto_retry: true,
      retry_count: 0,
      acr_registry_id: isMultiImage.value ? 0 : selectedAcrId.value
    }

    const response = await syncAPI.submitMockBatchSync(batchData)
    
    ElMessage.success('模拟批量同步任务已提交')
    emit('success', response)
    resetForm()
    
  } catch (error) {
    if (error === 'cancel' || error?.message === 'cancel') {
      return
    }
    if (error.errors) {
      return
    }
    ElMessage.error('提交模拟批量同步任务失败')
  } finally {
    mockLoading.value = false
  }
}

const resetForm = () => {
  batchFormRef.value?.resetFields()
  Object.assign(batchForm, {
    description: ''
  })
  doClearImages()
  inputMode.value = 'manual'
  const defaultAcr = acrList.value.find(item => item.is_default)
  selectedAcrId.value = defaultAcr ? defaultAcr.id : null
}

onUnmounted(() => {
  if (parseDebounceTimer) {
    clearTimeout(parseDebounceTimer)
    parseDebounceTimer = null
  }
  if (suggestTimer) {
    clearTimeout(suggestTimer)
    suggestTimer = null
  }
})
</script>

<style scoped>
.batch-sync-form {
  width: 100%;
}

.image-input-section {
  width: 100%;
}

.input-mode-selector {
  margin-bottom: 16px;
}

.manual-input {
  margin-bottom: 16px;
}

.input-tips {
  margin-top: 8px;
}

.file-input {
  margin-bottom: 16px;
}

.image-preview {
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  padding: 12px;
  background-color: #fafafa;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  font-weight: 500;
}

.image-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.image-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  background-color: #fafafa;
}

.image-info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.image-tag {
  margin: 0;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.multi-acr-hint {
  margin: -8px 0 16px 120px;
  color: #909399;
  font-size: 13px;
  line-height: 1.5;
}

.force-override-warning {
  margin: -8px 0 16px 120px;
  color: #f56c6c;
  font-size: 13px;
  line-height: 1.5;
}
</style>
