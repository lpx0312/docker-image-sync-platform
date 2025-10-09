<template>
  <div class="batch-sync-form">
    <el-form 
      ref="batchFormRef" 
      :model="batchForm" 
      :rules="batchRules" 
      label-width="120px"
    >
      <el-form-item label="任务描述" prop="description">
        <el-input
          v-model="batchForm.description"
          placeholder="请输入批量同步任务的描述"
          maxlength="200"
          show-word-limit
        />
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
              placeholder="请输入镜像列表，每行一个镜像，格式如下：&#10;nginx:latest&#10;redis:6.2&#10;mysql:8.0&#10;ubuntu:20.04"
              @input="parseImageInput"
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
              <span>解析到的镜像列表 ({{ parsedImages.length }} 个)</span>
              <el-button size="small" text @click="clearImages">清空</el-button>
            </div>
            <div class="image-list">
              <el-tag
                v-for="(image, index) in parsedImages"
                :key="index"
                closable
                @close="removeImage(index)"
                class="image-tag"
              >
                {{ image }}
              </el-tag>
            </div>
          </div>
        </div>
      </el-form-item>

      <el-form-item label="并发数量" prop="maxConcurrent">
        <el-input-number
          v-model="batchForm.maxConcurrent"
          :min="1"
          :max="10"
          :step="1"
          style="width: 200px"
        />
        <div class="form-tip">
          同时处理的镜像数量，建议设置为1-5，避免过高导致资源占用
        </div>
      </el-form-item>

      <el-form-item label="失败重试" prop="autoRetry">
        <el-switch
          v-model="batchForm.autoRetry"
          active-text="启用"
          inactive-text="禁用"
        />
        <span class="form-tip" style="margin-left: 10px">
          启用后，失败的镜像将自动重试
        </span>
      </el-form-item>

      <el-form-item v-if="batchForm.autoRetry" label="重试次数" prop="maxRetries">
        <el-input-number
          v-model="batchForm.maxRetries"
          :min="1"
          :max="5"
          :step="1"
          style="width: 200px"
        />
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
        <el-button @click="resetForm">
          <el-icon><RefreshLeft /></el-icon>
          重置
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Upload, RefreshLeft } from '@element-plus/icons-vue'
import { syncAPI } from '@/api'

// 组件事件
const emit = defineEmits(['success'])

// 表单引用
const batchFormRef = ref()
const uploadRef = ref()

// 输入模式
const inputMode = ref('manual')

// 表单数据
const batchForm = reactive({
  description: '',
  maxConcurrent: 3,
  autoRetry: true,
  maxRetries: 2
})

// 镜像输入
const imageInput = ref('')
const parsedImages = ref([])
const loading = ref(false)

// 表单验证规则
const batchRules = {
  description: [
    { required: true, message: '请输入任务描述', trigger: 'blur' }
  ],
  maxConcurrent: [
    { required: true, message: '请设置并发数量', trigger: 'blur' },
    { type: 'number', min: 1, max: 10, message: '并发数量必须在1-10之间', trigger: 'blur' }
  ],
  maxRetries: [
    { type: 'number', min: 1, max: 5, message: '重试次数必须在1-5之间', trigger: 'blur' }
  ]
}

// 解析镜像输入
const parseImageInput = () => {
  const lines = imageInput.value.split('\n')
  const images = []
  
  for (const line of lines) {
    const trimmed = line.trim()
    if (trimmed && isValidImageFormat(trimmed)) {
      if (!images.includes(trimmed)) {
        images.push(trimmed)
      }
    }
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

// 移除单个镜像
const removeImage = (index) => {
  parsedImages.value.splice(index, 1)
  imageInput.value = parsedImages.value.join('\n')
}

// 清空镜像列表
const clearImages = () => {
  parsedImages.value = []
  imageInput.value = ''
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
}

// 提交批量同步
const submitBatchSync = async () => {
  try {
    await batchFormRef.value.validate()
    
    if (parsedImages.value.length === 0) {
      ElMessage.warning('请至少添加一个镜像')
      return
    }
    
    loading.value = true
    
    // 构建批量同步请求
    const batchData = {
      description: batchForm.description,
      images: parsedImages.value.map(image => {
        const [imageName, tag] = image.split(':')
        return {
          image: imageName,
          tag: tag || 'latest',
          architecture: 'amd64', // 默认架构
          priority: 1,
          max_retries: batchForm.autoRetry ? batchForm.maxRetries : 0
        }
      }),
      max_concurrent: batchForm.maxConcurrent,
      auto_retry: batchForm.autoRetry
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

// 重置表单
const resetForm = () => {
  batchFormRef.value?.resetFields()
  Object.assign(batchForm, {
    description: '',
    maxConcurrent: 3,
    autoRetry: true,
    maxRetries: 2
  })
  clearImages()
  inputMode.value = 'manual'
}
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
  flex-wrap: wrap;
  gap: 8px;
}

.image-tag {
  margin: 0;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}
</style>