<!--
  Git操作测试结果弹窗组件

  功能说明：
  - 显示GitHub代码拉取和提交测试的详细结果
  - 包含拉取、提交、推送操作的成功状态和耗时
  - 提供操作步骤的详细展示
  - 支持错误信息的友好展示

  @author Docker Image Sync Platform
  @version 1.0.0
-->

<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="GitHub代码操作测试结果"
    width="600px"
    :close-on-click-modal="false"
    :close-on-press-escape="true"
    @close="handleClose"
  >
    <div class="test-result-container">
      <!-- 总体结果 -->
      <div class="overall-result">
        <el-alert
          :title="overallTitle"
          :type="overallType"
          :description="overallDescription"
          show-icon
          :closable="false"
        />
      </div>

      <!-- 测试步骤结果 -->
      <div class="test-steps" v-if="testResult">
        <h4 class="steps-title">测试步骤详情</h4>

        <!-- 步骤1: 拉取images.txt -->
        <div class="step-item">
          <div class="step-header">
            <el-icon class="step-icon" :class="getStepIconClass(testResult.pull_success)">
              <Download v-if="testResult.pull_success" />
              <Close v-else />
            </el-icon>
            <span class="step-name">1. 拉取images.txt文件</span>
            <el-tag :type="testResult.pull_success ? 'success' : 'danger'" size="small">
              {{ testResult.pull_success ? '成功' : '失败' }}
            </el-tag>
          </div>
          <div class="step-details">
            <span class="step-time">耗时: {{ formatTime(testResult.pull_time) }}</span>
          </div>
        </div>

        <!-- 步骤2: 提交测试内容 -->
        <div class="step-item" v-if="testResult.pull_success">
          <div class="step-header">
            <el-icon class="step-icon" :class="getStepIconClass(testResult.commit_success)">
              <Upload v-if="testResult.commit_success" />
              <Close v-else />
            </el-icon>
            <span class="step-name">2. 提交测试内容到images.txt</span>
            <el-tag :type="testResult.commit_success ? 'success' : 'danger'" size="small">
              {{ testResult.commit_success ? '成功' : '失败' }}
            </el-tag>
          </div>
          <div class="step-details">
            <span class="step-time">耗时: {{ formatTime(testResult.commit_time) }}</span>
            <span v-if="testResult.commit_sha" class="commit-sha">
              提交SHA: {{ testResult.commit_sha.substring(0, 8) }}
            </span>
          </div>
        </div>

        <!-- 步骤3: 验证提交内容 -->
        <div class="step-item" v-if="testResult.commit_success">
          <div class="step-header">
            <el-icon class="step-icon" :class="getStepIconClass(testResult.test_images_txt)">
              <Check v-if="testResult.test_images_txt" />
              <Close v-else />
            </el-icon>
            <span class="step-name">3. 验证提交内容</span>
            <el-tag :type="testResult.test_images_txt ? 'success' : 'danger'" size="small">
              {{ testResult.test_images_txt ? '验证通过' : '验证失败' }}
            </el-tag>
          </div>
          <div class="step-details">
            <span class="step-time">耗时: {{ formatTime(testResult.push_time) }}</span>
          </div>
        </div>
      </div>

      <!-- 错误信息 -->
      <div class="error-section" v-if="testResult && testResult.error_message">
        <h4 class="error-title">错误信息</h4>
        <el-alert
          title="操作失败"
          type="error"
          :description="testResult.error_message"
          show-icon
          :closable="false"
        />
      </div>

      <!-- 统计信息 -->
      <div class="statistics-section" v-if="testResult">
        <h4 class="statistics-title">统计信息</h4>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="总耗时">
            {{ formatTime(testResult.total_time) }}
          </el-descriptions-item>
          <el-descriptions-item label="拉取耗时">
            {{ formatTime(testResult.pull_time) }}
          </el-descriptions-item>
          <el-descriptions-item label="提交耗时">
            {{ formatTime(testResult.commit_time) }}
          </el-descriptions-item>
          <el-descriptions-item label="验证耗时">
            {{ formatTime(testResult.push_time) }}
          </el-descriptions-item>
        </el-descriptions>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">关闭</el-button>
        <el-button
          type="primary"
          v-if="testResult && testResult.commit_success"
          @click="viewCommit"
        >
          查看提交
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { Download, Upload, Check, Close } from '@element-plus/icons-vue'

// Props
const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  testResult: {
    type: Object,
    default: null
  }
})

// Emits
const emit = defineEmits(['update:visible', 'close', 'view-commit'])

// 计算属性
const overallTitle = computed(() => {
  if (!props.testResult) return '测试结果'

  console.log('计算overallTitle, testResult:', props.testResult)
  const pullSuccess = props.testResult.pull_success
  const commitSuccess = props.testResult.commit_success
  const testImagesTxt = props.testResult.test_images_txt
  const errorMessage = props.testResult.error_message

  console.log('各步骤状态:', { pullSuccess, commitSuccess, testImagesTxt, errorMessage })

  if (pullSuccess && commitSuccess && testImagesTxt) {
    console.log('返回: 测试全部通过')
    return '测试全部通过'
  } else if (errorMessage) {
    console.log('返回: 测试部分失败')
    return '测试部分失败'
  } else {
    console.log('返回: 测试完成')
    return '测试完成'
  }
})

const overallType = computed(() => {
  if (!props.testResult) return 'info'

  if (props.testResult.pull_success &&
      props.testResult.commit_success &&
      props.testResult.test_images_txt) {
    return 'success'
  } else if (props.testResult.error_message) {
    return 'error'
  } else {
    return 'warning'
  }
})

const overallDescription = computed(() => {
  if (!props.testResult) return ''

  if (props.testResult.pull_success &&
      props.testResult.commit_success &&
      props.testResult.test_images_txt) {
    return `GitHub代码拉取和提交操作全部成功，总耗时: ${formatTime(props.testResult.total_time)}`
  } else {
    return 'GitHub代码操作测试完成，请查看详细结果'
  }
})

// 方法
const formatTime = (milliseconds) => {
  if (!milliseconds || milliseconds === 0) return '-'

  if (milliseconds < 1000) {
    return `${milliseconds} ms`
  } else {
    return `${(milliseconds / 1000).toFixed(2)} s`
  }
}

const getStepIconClass = (success) => {
  return success ? 'step-success' : 'step-error'
}

const handleClose = () => {
  emit('update:visible', false)
  emit('close')
}

const viewCommit = () => {
  if (props.testResult && props.testResult.commit_sha) {
    emit('view-commit', props.testResult.commit_sha)
  }
}

// 监听visible变化
watch(() => props.visible, (newVal) => {
  if (newVal) {
    console.log('Git测试结果弹窗打开', props.testResult)
    console.log('test_images_txt值:', props.testResult?.test_images_txt)
    console.log('pull_success值:', props.testResult?.pull_success)
    console.log('commit_success值:', props.testResult?.commit_success)
    console.log('push_success值:', props.testResult?.push_success)
    console.log('error_message值:', props.testResult?.error_message)
  }
})
</script>

<style scoped>
.test-result-container {
  padding: 0;
}

.overall-result {
  margin-bottom: 24px;
}

.steps-title {
  margin: 24px 0 16px 0;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.step-item {
  margin-bottom: 16px;
  padding: 16px;
  background-color: #f8f9fa;
  border-radius: 8px;
  border-left: 4px solid #e4e7ed;
  transition: all 0.3s ease;
}

.step-item:hover {
  background-color: #f0f2f5;
  border-left-color: #409eff;
}

.step-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.step-icon {
  font-size: 18px;
}

.step-success {
  color: #67c23a;
}

.step-error {
  color: #f56c6c;
}

.step-name {
  flex: 1;
  font-weight: 500;
  color: #303133;
}

.step-details {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-left: 26px;
  font-size: 13px;
  color: #606266;
}

.step-time {
  color: #909399;
}

.commit-sha {
  background-color: #e1f3d8;
  color: #67c23a;
  padding: 2px 8px;
  border-radius: 4px;
  font-family: 'Courier New', monospace;
  font-size: 12px;
}

.error-section {
  margin: 24px 0;
}

.error-title {
  margin-bottom: 12px;
  font-size: 16px;
  font-weight: 600;
  color: #f56c6c;
}

.statistics-section {
  margin-top: 24px;
}

.statistics-title {
  margin-bottom: 12px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .step-details {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
}
</style>