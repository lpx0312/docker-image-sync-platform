<!--
  单个镜像同步表单组件
  
  功能说明：
  - 提供单个Docker镜像的同步配置界面
  - 支持源镜像地址输入和验证
  - 支持架构选择 (amd64/arm64)
  - 支持同步说明描述
  - 集成表单验证和提交处理
  
  使用场景：
  - 用户需要同步单个镜像时使用
  - 提供简洁的表单界面
  - 支持实时验证和错误提示
  
  事件：
  - @success: 同步任务提交成功时触发，传递任务信息
  
  依赖：
  - Element Plus UI组件库
  - Pinia状态管理 (syncStore)
  
  @author Docker Image Sync Platform
  @version 1.0.0
-->
<template>
  <div class="single-sync-form">
    <!-- 同步表单 -->
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
/**
 * 单个镜像同步表单组件逻辑
 * 
 * 主要功能：
 * - 表单数据管理和验证
 * - 同步任务提交处理
 * - 用户交互和状态反馈
 */

import { ref, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { Upload, RefreshLeft } from '@element-plus/icons-vue'
import { useSyncStore } from '@/stores/sync'

/**
 * 组件事件定义
 * @event success - 同步任务提交成功时触发
 */
const emit = defineEmits(['success'])

/**
 * 同步状态管理store
 * 用于处理同步任务的提交和状态跟踪
 */
const syncStore = useSyncStore()

/**
 * 表单DOM引用
 * 用于表单验证和重置操作
 */
const syncFormRef = ref()

/**
 * 表单数据模型
 * @property {string} sourceImage - 源镜像地址
 * @property {string} architecture - 目标架构 (amd64/arm64)
 * @property {string} description - 同步说明描述
 */
const syncForm = reactive({
  sourceImage: '',
  description: ''
})

/**
 * 表单验证规则配置
 * 
 * 验证规则：
 * - sourceImage: 必填，格式验证 (镜像名:标签)
 * - architecture: 可选，默认amd64
 * - description: 可选，最大500字符
 */
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

/**
 * 提交同步任务
 * 
 * 处理流程：
 * 1. 验证表单数据
 * 2. 构造同步请求数据
 * 3. 调用store提交同步任务
 * 4. 处理成功/失败反馈
 * 5. 通知父组件任务状态
 * 
 * @async
 * @throws {Error} 表单验证失败或网络请求失败
 */
const submitSync = async () => {
  try {
    await syncFormRef.value.validate()
    
    const syncData = {
      images: [syncForm.sourceImage],
      description: syncForm.description
    }
    
    const result = await syncStore.submitSync(syncData)
    
    // 成功提示
    ElMessage.success('同步任务已提交')
    
    // 通知父组件同步成功
    emit('success', syncStore.currentTask)
    
  } catch (error) {
    console.error('Submit sync error:', error)
    if (error.errors) {
      // 表单验证错误，不显示额外提示
      return
    }
    // 网络或其他错误
    ElMessage.error('提交同步任务失败')
  }
}

/**
 * 重置表单数据
 * 
 * 功能：
 * - 清空所有表单字段
 * - 重置验证状态
 * - 恢复默认值
 */
const resetForm = () => {
  // 重置表单验证状态
  syncFormRef.value.resetFields()
  // 重置表单数据到初始状态
  Object.assign(syncForm, {
    sourceImage: '',
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