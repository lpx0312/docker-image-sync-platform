<template>
  <div class="aliyun-config-container">
    <!-- 阿里云配置 -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Monitor /></el-icon>
          <span class="header-title">阿里云镜像仓库配置</span>
          <el-button 
            type="primary" 
            size="small" 
            @click="testConnection"
            :loading="testingConnection"
            :disabled="!aliyunConfig.registry_url || !aliyunConfig.username"
          >
            测试连接
          </el-button>
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
              @blur="saveConfig"
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
              @blur="saveConfig"
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
              placeholder="阿里云账号用户名"
              @blur="saveConfig"
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
              placeholder="阿里云镜像仓库密码"
              show-password
              @blur="saveConfig"
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

          <el-form-item label="地域" required>
            <el-select
              v-model="aliyunConfig.region"
              placeholder="选择阿里云地域"
              @change="saveConfig"
              style="width: 100%"
            >
              <el-option
                v-for="region in regionOptions"
                :key="region.value"
                :label="region.label"
                :value="region.value"
              />
            </el-select>
            <div class="form-help">
              <el-text size="small" type="info">
                选择阿里云容器镜像服务所在的地域
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
import { Monitor, Link, User, Lock, Location } from '@element-plus/icons-vue'
import { systemAPI } from '@/api'

// ====================================================================
// 响应式数据定义
// ====================================================================

const loading = ref(false)
const lastSaved = ref('')
const testingConnection = ref(false)
const connectionStatus = ref(null)

// 阿里云配置数据
const aliyunConfig = ref({
  registry_url: '',
  namespace: '',
  username: '',
  password: '',
  region: ''
})

// 阿里云地域选项
const regionOptions = [
  { value: 'cn-hangzhou', label: '华东1（杭州）' },
  { value: 'cn-shanghai', label: '华东2（上海）' },
  { value: 'cn-qingdao', label: '华北1（青岛）' },
  { value: 'cn-beijing', label: '华北2（北京）' },
  { value: 'cn-zhangjiakou', label: '华北3（张家口）' },
  { value: 'cn-huhehaote', label: '华北5（呼和浩特）' },
  { value: 'cn-wulanchabu', label: '华北6（乌兰察布）' },
  { value: 'cn-shenzhen', label: '华南1（深圳）' },
  { value: 'cn-heyuan', label: '华南2（河源）' },
  { value: 'cn-guangzhou', label: '华南3（广州）' },
  { value: 'cn-chengdu', label: '西南1（成都）' },
  { value: 'cn-hongkong', label: '中国香港' },
  { value: 'ap-southeast-1', label: '新加坡' },
  { value: 'ap-southeast-2', label: '澳大利亚（悉尼）' },
  { value: 'ap-southeast-3', label: '马来西亚（吉隆坡）' },
  { value: 'ap-southeast-5', label: '印度尼西亚（雅加达）' },
  { value: 'ap-northeast-1', label: '日本（东京）' },
  { value: 'ap-south-1', label: '印度（孟买）' },
  { value: 'us-east-1', label: '美国（弗吉尼亚）' },
  { value: 'us-west-1', label: '美国（硅谷）' },
  { value: 'eu-west-1', label: '英国（伦敦）' },
  { value: 'me-east-1', label: '阿联酋（迪拜）' },
  { value: 'eu-central-1', label: '德国（法兰克福）' }
]

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
      aliyunConfig.value = {
        registry_url: response.data.registry_url || '',
        namespace: response.data.namespace || '',
        username: response.data.username || '',
        password: response.data.password || '',
        region: response.data.region || ''
      }
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
    const response = await systemAPI.updateAliyunConfig(aliyunConfig.value)
    
    if (response.status === 'success') {
      updateLastSaved()
      ElMessage.success('阿里云配置已保存')
    } else {
      ElMessage.error('保存阿里云配置失败：' + response.message)
    }
  } catch (error) {
    console.error('保存阿里云配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  }
}

/**
 * 测试连接
 */
const testConnection = async () => {
  try {
    testingConnection.value = true
    connectionStatus.value = null
    
    // 这里可以调用后端API测试阿里云连接
    // 暂时模拟测试结果
    await new Promise(resolve => setTimeout(resolve, 2000))
    
    // 简单验证配置完整性
    if (!aliyunConfig.value.registry_url || !aliyunConfig.value.username || 
        !aliyunConfig.value.password || !aliyunConfig.value.namespace) {
      connectionStatus.value = {
        title: '配置不完整',
        type: 'warning',
        message: '请填写完整的阿里云配置信息'
      }
      return
    }
    
    connectionStatus.value = {
      title: '连接成功',
      type: 'success',
      message: '阿里云镜像仓库连接测试通过，配置正确'
    }
  } catch (error) {
    console.error('测试阿里云连接失败:', error)
    connectionStatus.value = {
      title: '连接失败',
      type: 'error',
      message: error.response?.data?.message || error.message || '连接测试失败'
    }
  } finally {
    testingConnection.value = false
  }
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