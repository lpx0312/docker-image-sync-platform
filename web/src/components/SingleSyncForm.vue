<template>
  <div class="single-sync-form">
    <el-form 
      ref="syncFormRef" 
      :model="syncForm" 
      :rules="syncRules" 
      label-width="120px"
    >
      <el-form-item label="源镜像地址" prop="sourceImage">
        <el-input
          v-model="syncForm.sourceImage"
          placeholder="例如: nginx:latest 或 docker.io/library/nginx:latest"
          clearable
        />
        <div class="form-tip">
          支持Docker Hub、Quay.io等公共镜像仓库的镜像
        </div>
      </el-form-item>

      <el-form-item label="架构选择" prop="architecture">
        <el-select
          v-model="syncForm.architecture"
          placeholder="请选择架构"
          style="width: 100%"
        >
          <el-option label="amd64" value="amd64" />
          <el-option label="arm64" value="arm64" />
        </el-select>
        <div class="form-tip">
          选择镜像的目标架构，默认为amd64
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
import { ref, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { Upload, RefreshLeft } from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'

// 组件事件
const emit = defineEmits(['success'])

// 状态管理
const syncStore = useSyncStore()

// 表单引用
const syncFormRef = ref()

// 表单数据
const syncForm = reactive({
  sourceImage: '',
  architecture: 'amd64',
  description: ''
})

// 表单验证规则
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

// 提交同步
const submitSync = async () => {
  try {
    console.log('Submitting sync form...')
    await syncFormRef.value.validate()
    
    const syncData = {
      images: [syncForm.sourceImage],
      architecture: syncForm.architecture
    }
    
    console.log('Sync data:', syncData)
    const result = await syncStore.submitSync(syncData)
    console.log('Sync submitted, result:', result)
    console.log('Current task after submit:', syncStore.currentTask)
    
    ElMessage.success('同步任务已提交')
    
    // 通知父组件
    emit('success', syncStore.currentTask)
    
  } catch (error) {
    console.error('Submit sync error:', error)
    if (error.errors) {
      // 表单验证错误
      return
    }
    ElMessage.error('提交同步任务失败')
  }
}

// 重置表单
const resetForm = () => {
  syncFormRef.value.resetFields()
  Object.assign(syncForm, {
    sourceImage: '',
    architecture: 'amd64',
    description: ''
  })
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
</style>