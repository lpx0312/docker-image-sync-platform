<!--
  批量镜像同步表单组件
  
  功能说明：
  - 提供批量Docker镜像的同步配置界面
  - 支持手动输入和文件导入两种方式
  - 支持镜像列表预览和验证
  - 支持架构选择和同步选项配置
  - 提供模拟同步和实际同步功能
  - 集成进度跟踪和状态显示
  
  输入方式：
  - 手动输入: 文本框输入，每行一个镜像
  - 文件导入: 支持.txt和.csv文件上传
  
  功能特性：
  - 镜像格式验证和去重
  - 实时预览和统计
  - 模拟同步测试
  - 批量任务进度跟踪
  - 错误处理和重试机制
  
  事件：
  - @success: 批量同步任务提交成功时触发
  - @progress: 同步进度更新时触发
  
  依赖：
  - Element Plus UI组件库
  - Pinia状态管理 (syncStore)
  
  @author Docker Image Sync Platform
  @version 1.0.0
-->
<template>
  <div class="batch-sync-form">
    <el-form 
      ref="batchFormRef" 
      :model="batchForm" 
      :rules="batchRules" 
      label-width="120px"
    >
      <el-form-item label="目标 ACR">
        <el-select
          v-model="selectedAcrId"
          placeholder="选择目标 ACR"
          style="width: 100%"
        >
          <el-option
            v-for="item in acrList"
            :key="item.id"
            :label="item.namespace"
            :value="item.id"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="镜像列表" prop="images">
        <div class="image-input-section">
          <!-- 输入方式选择 -->
          <el-radio-group v-model="inputMode" class="input-mode-selector">
            <el-radio-button label="manual">手动输入</el-radio-button>
            <el-radio-button label="file">文件导入</el-radio-button>
          </el-radio-group>

          <!-- 手动输入模式 -->
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

          <!-- 文件导入模式 -->
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

          <!-- 镜像列表预览 -->
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
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Upload, RefreshLeft } from '@element-plus/icons-vue'
import { syncAPI, acrRegistryAPI } from '@/api'

// 组件事件
const emit = defineEmits(['success'])

// 表单引用
const batchFormRef = ref()
const uploadRef = ref()

// 输入模式
const inputMode = ref('manual')

// ACR 列表和选中的 ACR
const acrList = ref([])
const selectedAcrId = ref(null)

// 表单数据（并发与重试由服务端 config.yaml 的 sync 段决定）
const batchForm = reactive({
  description: ''
})

// 镜像输入
const imageInput = ref('')
const parsedImages = ref([])
const loading = ref(false)
const mockLoading = ref(false)
let parseDebounceTimer = null

const batchRules = {}

// 组件挂载时加载 ACR 列表
onMounted(async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    if (response && response.status === 'success') {
      acrList.value = response.data || []
      // 默认选中默认 ACR
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        selectedAcrId.value = defaultAcr.id
      }
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
  }
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

// 带防抖的解析入口，避免每次按键都弹校验错误
const debouncedParseImageInput = () => {
  if (parseDebounceTimer) {
    clearTimeout(parseDebounceTimer)
  }
  parseDebounceTimer = setTimeout(() => {
    parseImageInput()
  }, 500)
}

// 解析镜像输入（仅 image:tag，去重保序）
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
}

// 验证镜像格式
const isValidImageFormat = (image) => {
  // 基本的镜像格式验证
  const imageRegex = /^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$/
  return imageRegex.test(image)
}

// 处理文件变化
const handleFileChange = (file) => {
  const reader = new FileReader()
  reader.onload = (e) => {
    const content = e.target.result
    parseFileContent(content, file.name)
  }
  reader.readAsText(file.raw)
}

// 解析文件内容
const parseFileContent = (content, fileName) => {
  const images = []
  
  if (fileName.endsWith('.csv')) {
    // CSV格式解析
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
    // 文本格式解析
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
}

// 处理文件移除
const handleFileRemove = () => {
  parsedImages.value = []
  imageInput.value = ''
}

const rebuildImageInput = () => {
  imageInput.value = parsedImages.value.join('\n')
}

const removeImage = (index) => {
  parsedImages.value.splice(index, 1)
  rebuildImageInput()
}

// 清空镜像列表（内部实现）
const doClearImages = () => {
  parsedImages.value = []
  imageInput.value = ''
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
}

// 清空镜像列表（带二次确认）
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

// 提交批量同步
const submitBatchSync = async () => {
  try {
    await batchFormRef.value.validate()

    if (parsedImages.value.length === 0) {
      ElMessage.warning('请至少添加一个镜像')
      return
    }

    loading.value = true

    const batchData = {
      images: buildBatchImageItems(),
      max_concurrent: 0,
      auto_retry: true,
      retry_count: 0,
      acr_registry_id: selectedAcrId.value
    }

    const response = await syncAPI.submitBatchSync(batchData)
    
    ElMessage.success('批量同步任务已提交')
    
    // 通知父组件
    emit('success', response)
    
    // 重置表单
    resetForm()
    
  } catch (error) {
    if (error.errors) {
      // 表单验证错误
      return
    }
    ElMessage.error('提交批量同步任务失败')
  } finally {
    loading.value = false
  }
}

// 提交模拟批量同步
const submitMockBatchSync = async () => {
  try {
    await batchFormRef.value.validate()

    if (parsedImages.value.length === 0) {
      ElMessage.warning('请至少添加一个镜像')
      return
    }

    mockLoading.value = true

    const batchData = {
      images: buildBatchImageItems(),
      max_concurrent: 0,
      auto_retry: true,
      retry_count: 0,
      acr_registry_id: selectedAcrId.value
    }

    const response = await syncAPI.submitMockBatchSync(batchData)
    
    ElMessage.success('模拟批量同步任务已提交')
    
    // 通知父组件
    emit('success', response)
    
    // 重置表单
    resetForm()
    
  } catch (error) {
    if (error.errors) {
      // 表单验证错误
      return
    }
    ElMessage.error('提交模拟批量同步任务失败')
  } finally {
    mockLoading.value = false
  }
}

// 重置表单
const resetForm = () => {
  batchFormRef.value?.resetFields()
  Object.assign(batchForm, {
    description: ''
  })
  doClearImages()
  inputMode.value = 'manual'
  // 重置为默认 ACR
  const defaultAcr = acrList.value.find(item => item.is_default)
  selectedAcrId.value = defaultAcr ? defaultAcr.id : null
}

onUnmounted(() => {
  if (parseDebounceTimer) {
    clearTimeout(parseDebounceTimer)
    parseDebounceTimer = null
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
</style>