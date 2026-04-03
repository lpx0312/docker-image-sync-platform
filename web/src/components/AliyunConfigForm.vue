<template>
  <div class="aliyun-config-container">
    <!-- 阿里云配置 -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Monitor /></el-icon>
          <span class="header-title">阿里云镜像仓库配置</span>
          <div class="header-actions">
            <el-button
              type="success"
              size="small"
              @click="saveConfig"
              :loading="savingConfig"
              :disabled="!isConfigChanged"
            >
              保存并测试配置
            </el-button>
          </div>
        </div>
      </template>

      <div class="config-section">
        <div class="section-description">
          <p>配置阿里云容器镜像服务，用于存储和管理Docker镜像。</p>
        </div>

        <el-form :model="aliyunConfig" label-width="120px" :disabled="loading">
          <el-form-item label="镜像仓库URL" required>
            <el-input
              v-model="aliyunConfig.registry_url"
              placeholder="registry.cn-hangzhou.aliyuncs.com"
              @input="markConfigChanged"
            >
              <template #prefix>
                <el-icon><Link /></el-icon>
              </template>
            </el-input>
            <div class="form-help">
              <el-text size="small" type="info">
                阿里云容器镜像服务的注册表地址，例如：registry.cn-hangzhou.aliyuncs.com
              </el-text>
            </div>
          </el-form-item>

          <el-form-item label="命名空间" required>
            <el-input
              v-model="aliyunConfig.namespace"
              placeholder="your-namespace"
              @input="markConfigChanged"
            >
              <template #prefix>
                <el-icon><Location /></el-icon>
              </template>
            </el-input>
            <div class="form-help">
              <el-text size="small" type="info">
                阿里云镜像仓库的命名空间，用于组织和管理镜像
              </el-text>
            </div>
          </el-form-item>

          <el-form-item label="用户名" required>
            <el-input
              v-model="aliyunConfig.username"
              placeholder="阿里云用户名"
              @input="markConfigChanged"
            >
              <template #prefix>
                <el-icon><User /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item label="密码" required>
            <el-input
              v-model="aliyunConfig.password"
              type="password"
              placeholder="阿里云密码"
              show-password
              @input="markConfigChanged"
            >
              <template #prefix>
                <el-icon><Lock /></el-icon>
              </template>
            </el-input>
            <div class="form-help">
              <el-text size="small" type="info">
                在阿里云容器镜像服务控制台中设置的访问凭证密码
              </el-text>
            </div>
          </el-form-item>

          </el-form>

        <!-- 连接状态显示 -->
        <div v-if="connectionStatus" class="connection-status">
          <el-alert
            :title="connectionStatus.title"
            :type="connectionStatus.type"
            :description="connectionStatus.message"
            show-icon
            :closable="false"
          />
        </div>
      </div>
    </el-card>

    <!-- 配置状态显示 -->
    <div class="config-status" v-if="lastSaved">
      <el-icon class="status-icon"><CircleCheck /></el-icon>
      <span class="status-text">配置已保存 - {{ lastSaved }}</span>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Monitor, Link, User, Lock, Location, CircleCheck } from '@element-plus/icons-vue'
import { systemAPI } from '@/api'

// ====================================================================
// 响应式数据定义
// ====================================================================

const loading = ref(false)
const lastSaved = ref('')
const connectionStatus = ref(null)
const savingConfig = ref(false)
const isConfigChanged = ref(false)
const originalConfig = ref({})

// 阿里云配置数据
const aliyunConfig = ref({
  registry_url: '',
  namespace: '',
  username: '',
  password: ''
})


// ====================================================================
// 生命周期钩子
// ====================================================================

onMounted(() => {
  loadConfig()
})

// ====================================================================
// 方法定义
// ====================================================================

/**
 * 加载阿里云配置
 */
const loadConfig = async () => {
  try {
    loading.value = true
    
    const response = await systemAPI.getAliyunConfig()
    if (response.status === 'success' && response.data) {
      const configData = {
        registry_url: response.data.registry_url || '',
        namespace: response.data.namespace || '',
        username: response.data.username || '',
        password: response.data.password || ''
      }
      aliyunConfig.value = { ...configData }
      originalConfig.value = { ...configData }
      isConfigChanged.value = false
    }
  } catch (error) {
    console.error('加载阿里云配置失败:', error)
    ElMessage.error('加载配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

/**
 * 保存阿里云配置
 */
const saveConfig = async () => {
  try {
    savingConfig.value = true
    connectionStatus.value = null

    // 验证必填字段
    if (!aliyunConfig.value.registry_url || !aliyunConfig.value.namespace ||
        !aliyunConfig.value.username || !aliyunConfig.value.password) {
      ElMessage.error('请填写完整的阿里云配置信息')
      return
    }

    // 检查密码是否为占位符
    if (aliyunConfig.value.password === '***') {
      ElMessage.error('请输入新的阿里云密码以进行连接测试')
      return
    }

    // 先测试连接
    ElMessage.info('正在测试阿里云镜像仓库连接...')

    // 准备测试配置数据
    const testConfig = {
      registry_url: aliyunConfig.value.registry_url,
      namespace: aliyunConfig.value.namespace,
      username: aliyunConfig.value.username,
      password: aliyunConfig.value.password
    }

    const testResponse = await systemAPI.testAliyunConnection(testConfig)

    if (testResponse.status !== 'success') {
      connectionStatus.value = {
        title: '连接测试失败',
        type: 'error',
        message: testResponse.message || '阿里云镜像仓库连接测试失败，请检查配置'
      }
      ElMessage.error('连接测试失败，配置未保存')
      return
    }

    // 连接测试通过，准备保存配置数据
    const configData = {
      registry: aliyunConfig.value.registry_url,
      namespace: aliyunConfig.value.namespace,
      username: aliyunConfig.value.username,
      password: aliyunConfig.value.password
    }

    // 如果密码是占位符则不发送
    if (configData.password === '***') {
      delete configData.password // 不发送占位符密码
    }

    const response = await systemAPI.updateAliyunConfig(configData)

    if (response.status === 'success') {
      // 更新原始配置
      originalConfig.value = { ...aliyunConfig.value }
      isConfigChanged.value = false
      updateLastSaved()

      // 显示成功状态
      connectionStatus.value = {
        title: '配置保存成功',
        type: 'success',
        message: '阿里云配置已保存并通过连接测试'
      }

      ElMessage.success('阿里云配置已保存并通过连接测试')
    } else {
      connectionStatus.value = {
        title: '保存失败',
        type: 'error',
        message: '连接测试通过，但保存配置失败：' + response.message
      }
      ElMessage.error('保存阿里云配置失败：' + response.message)
    }
  } catch (error) {
    console.error('保存阿里云配置失败:', error)
    connectionStatus.value = {
      title: '操作失败',
      type: 'error',
      message: error.response?.data?.message || error.message || '保存配置失败'
    }
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    savingConfig.value = false
  }
}

/**
 * 标记配置已变更
 */
const markConfigChanged = () => {
  isConfigChanged.value = true
}


/**
 * 更新最后保存时间
 */
const updateLastSaved = () => {
  const now = new Date()
  lastSaved.value = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}
</script>

<style scoped>
/* ====================================================================
   组件整体布局样式
   ==================================================================== */

.aliyun-config-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* ====================================================================
   配置卡片样式
   ==================================================================== */

.config-card {
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  transition: all 0.3s ease;
}

.config-card:hover {
  border-color: #409eff;
  transform: translateY(-2px);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #303133;
}

.header-icon {
  font-size: 18px;
  color: #409eff;
}

.header-title {
  font-size: 16px;
  flex: 1;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.config-section {
  padding: 4px 0;
}

.section-description {
  margin-bottom: 20px;
}

.section-description p {
  color: #606266;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}

/* ====================================================================
   表单样式
   ==================================================================== */

.form-help {
  margin-top: 4px;
}

.connection-status {
  margin-top: 16px;
}

/* ====================================================================
   配置状态样式
   ==================================================================== */

.config-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background-color: #f0f9ff;
  border: 1px solid #b3d8ff;
  border-radius: 6px;
}

.status-icon {
  color: #67c23a;
  font-size: 16px;
}

.status-text {
  color: #409eff;
  font-size: 13px;
}

/* ====================================================================
   响应式设计
   ==================================================================== */

@media (max-width: 768px) {
  .aliyun-config-container {
    gap: 16px;
  }
}
</style>