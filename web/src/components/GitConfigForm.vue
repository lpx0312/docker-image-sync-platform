<template>
  <div class="git-config-container">
    <!-- Git仓库类型选择 -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Connection /></el-icon>
          <span class="header-title">Git仓库类型</span>
        </div>
      </template>

      <div class="config-section">
        <el-radio-group 
          v-model="gitRepositoryType" 
          @change="handleRepositoryTypeChange"
          :disabled="loading"
        >
          <el-radio value="gitee">Gitee</el-radio>
          <el-radio value="github">GitHub</el-radio>
        </el-radio-group>
      </div>
    </el-card>

    <!-- Gitee配置 -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span class="header-title">Gitee配置</span>
          <div class="header-actions">
            <el-button 
              type="success" 
              size="small" 
              @click="saveGiteeConfig"
              :loading="savingConfig.gitee"
              :disabled="!isGiteeConfigChanged"
            >
              保存配置
            </el-button>
            <el-button 
              type="primary" 
              size="small" 
              @click="testConnection('gitee')"
              :loading="testingConnection.gitee"
            >
              测试连接
            </el-button>
          </div>
        </div>
      </template>

      <el-form :model="giteeConfig" label-width="120px" :disabled="loading">
        <el-form-item label="仓库URL" required>
          <el-input
            v-model="giteeConfig.repo_url"
            placeholder="https://gitee.com/username/repository.git"
            @input="markGiteeConfigChanged"
          />
        </el-form-item>

        <el-form-item label="用户名" required>
          <el-input
            v-model="giteeConfig.username"
            placeholder="Gitee用户名"
            @input="markGiteeConfigChanged"
          />
        </el-form-item>

        <el-form-item label="密码" required>
          <el-input
            v-model="giteeConfig.password"
            type="password"
            placeholder="Gitee密码或访问令牌"
            show-password
            @input="markGiteeConfigChanged"
          />
        </el-form-item>

        <el-form-item label="邮箱" required>
          <el-input
            v-model="giteeConfig.email"
            placeholder="Git提交邮箱"
            @input="markGiteeConfigChanged"
          />
        </el-form-item>

        <el-form-item label="分支" required>
          <el-input
            v-model="giteeConfig.branch"
            placeholder="Git分支名称，如：main"
            @input="markGiteeConfigChanged"
          />
        </el-form-item>

        <!-- API模式下不再需要本地路径配置 -->
        <!-- <el-form-item label="本地路径" required>
          <el-input
            v-model="giteeConfig.local_path"
            placeholder="本地仓库路径，如：/tmp/gitee-repo"
            @input="markGiteeConfigChanged"
          />
        </el-form-item> -->
      </el-form>

      <div v-if="connectionStatus.gitee" class="connection-status">
        <el-alert
          :title="connectionStatus.gitee.title"
          :type="connectionStatus.gitee.type"
          :description="connectionStatus.gitee.message"
          show-icon
          :closable="false"
        />
      </div>
    </el-card>

    <!-- GitHub配置 -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span class="header-title">GitHub配置</span>
          <div class="header-actions">
            <el-button
              type="warning"
              size="small"
              @click="testGitOperations('github')"
              :loading="testingGitOperations.github"
            >
              测试代码拉取和提交
            </el-button>
            <el-button
              type="success"
              size="small"
              @click="saveGitHubConfig"
              :loading="savingConfig.github"
              :disabled="!isGitHubConfigChanged"
            >
              保存配置
            </el-button>
            <el-button
              type="primary"
              size="small"
              @click="testConnection('github')"
              :loading="testingConnection.github"
            >
              测试连接
            </el-button>
          </div>
        </div>
      </template>

      <el-form :model="githubConfig" label-width="120px" :disabled="loading">
        <el-form-item label="仓库URL" required>
          <el-input
            v-model="githubConfig.repo_url"
            placeholder="https://github.com/username/repository.git"
            @input="markGitHubConfigChanged"
          />
        </el-form-item>

        <el-form-item label="用户名" required>
          <el-input
            v-model="githubConfig.username"
            placeholder="GitHub用户名"
            @input="markGitHubConfigChanged"
          />
        </el-form-item>

        <el-form-item label="访问令牌" required>
          <el-input
            v-model="githubConfig.token"
            type="password"
            placeholder="GitHub Personal Access Token"
            show-password
            @input="markGitHubConfigChanged"
          />
        </el-form-item>

        <el-form-item label="邮箱" required>
          <el-input
            v-model="githubConfig.email"
            placeholder="Git提交邮箱"
            @input="markGitHubConfigChanged"
          />
        </el-form-item>

        <el-form-item label="分支" required>
          <el-input
            v-model="githubConfig.branch"
            placeholder="Git分支名称，如：main"
            @input="markGitHubConfigChanged"
          />
        </el-form-item>

        <!-- API模式下不再需要本地路径配置 -->
        <!-- <el-form-item label="本地路径" required>
          <el-input
            v-model="githubConfig.local_path"
            placeholder="本地仓库路径，如：/tmp/github-repo"
            @input="markGitHubConfigChanged"
          />
        </el-form-item> -->
      </el-form>

      <div v-if="connectionStatus.github" class="connection-status">
        <el-alert
          :title="connectionStatus.github.title"
          :type="connectionStatus.github.type"
          :description="connectionStatus.github.message"
          show-icon
          :closable="false"
        />
      </div>
    </el-card>

    <div class="config-status" v-if="lastSaved">
      <span class="status-text">配置已保存 - {{ lastSaved }}</span>
    </div>

    <!-- Git操作测试结果弹窗 -->
    <GitTestResultDialog
      v-model:visible="testResultVisible"
      :test-result="testResultData"
      @close="handleTestResultClose"
      @view-commit="handleViewCommit"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection } from '@element-plus/icons-vue'
import { systemAPI } from '@/api'
import GitTestResultDialog from './GitTestResultDialog.vue'

const loading = ref(false)
const lastSaved = ref('')
const gitRepositoryType = ref('gitee')

// 保存状态管理
const savingConfig = ref({
  gitee: false,
  github: false
})

// 配置变更状态管理
const isGiteeConfigChanged = ref(false)
const isGitHubConfigChanged = ref(false)

// 原始配置备份，用于检测变更和密码处理
const originalGiteeConfig = ref({})
const originalGitHubConfig = ref({})

const giteeConfig = ref({
  repo_url: '',
  username: '',
  password: '',
  email: '',
  branch: ''
})

const githubConfig = ref({
  repo_url: '',
  username: '',
  token: '',
  email: '',
  branch: ''
})

const testingConnection = ref({
  gitee: false,
  github: false
})

const testingGitOperations = ref({
  gitee: false,
  github: false
})

const connectionStatus = ref({
  gitee: null,
  github: null
})

// Git操作测试相关
const testResultVisible = ref(false)
const testResultData = ref(null)

onMounted(() => {
  loadConfigs()
})

const loadConfigs = async () => {
  try {
    loading.value = true
    
    const repoResponse = await systemAPI.getGitRepositoryConfig()
    if (repoResponse.status === 'success') {
      gitRepositoryType.value = repoResponse.data.repository_type
    }
    
    const gitResponse = await systemAPI.getGitConfig()
    if (gitResponse.status === 'success') {
      const data = gitResponse.data
      
      if (data.gitee) {
        // 保存原始配置用于测试连接
        originalGiteeConfig.value = {
          repo_url: data.gitee.repo_url || '',
          username: data.gitee.username || '',
          password: data.gitee.password || '',
          email: data.gitee.email || '',
          branch: data.gitee.branch || ''
        }

        giteeConfig.value = {
          repo_url: data.gitee.repo_url || '',
          username: data.gitee.username || '',
          password: data.gitee.password || '',
          email: data.gitee.email || '',
          branch: data.gitee.branch || ''
        }
      }
      
      if (data.github) {
        // 保存原始配置用于测试连接
        originalGitHubConfig.value = {
          repo_url: data.github.repo_url || '',
          username: data.github.username || '',
          token: data.github.token || '',
          email: data.github.email || '',
          branch: data.github.branch || ''
        }

        githubConfig.value = {
          repo_url: data.github.repo_url || '',
          username: data.github.username || '',
          token: data.github.token || '',
          email: data.github.email || '',
          branch: data.github.branch || ''
        }
      }
    }
    
    // 重置变更状态
    isGiteeConfigChanged.value = false
    isGitHubConfigChanged.value = false
  } catch (error) {
    console.error('加载配置失败:', error)
    ElMessage.error('加载配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

const handleRepositoryTypeChange = async (newType) => {
  try {
    loading.value = true
    const response = await systemAPI.updateGitRepositoryConfig(newType)
    
    if (response.status === 'success') {
      updateLastSaved()
      ElMessage.success(`Git仓库类型已更新为 ${newType === 'gitee' ? 'Gitee' : 'GitHub'}`)
    } else {
      ElMessage.error('保存配置失败：' + response.message)
      await loadConfigs()
    }
  } catch (error) {
    console.error('更新Git仓库配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
    await loadConfigs()
  } finally {
    loading.value = false
  }
}

// 标记配置变更
const markGiteeConfigChanged = () => {
  isGiteeConfigChanged.value = true
}

const markGitHubConfigChanged = () => {
  isGitHubConfigChanged.value = true
}

const saveGiteeConfig = async () => {
  try {
    savingConfig.value.gitee = true
    const response = await systemAPI.updateGiteeConfig(giteeConfig.value)
    
    if (response.status === 'success') {
      // 更新原始配置
      originalGiteeConfig.value = { ...giteeConfig.value }
      isGiteeConfigChanged.value = false
      updateLastSaved()
      ElMessage.success('Gitee配置已保存')
    } else {
      ElMessage.error('保存Gitee配置失败：' + response.message)
    }
  } catch (error) {
    console.error('保存Gitee配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    savingConfig.value.gitee = false
  }
}

const saveGitHubConfig = async () => {
  try {
    savingConfig.value.github = true
    const response = await systemAPI.updateGitHubConfig(githubConfig.value)
    
    if (response.status === 'success') {
      // 更新原始配置
      originalGitHubConfig.value = { ...githubConfig.value }
      isGitHubConfigChanged.value = false
      updateLastSaved()
      ElMessage.success('GitHub配置已保存')
    } else {
      ElMessage.error('保存GitHub配置失败：' + response.message)
    }
  } catch (error) {
    console.error('保存GitHub配置失败:', error)
    ElMessage.error('保存配置失败：' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    savingConfig.value.github = false
  }
}

const testConnection = async (type) => {
  try {
    testingConnection.value[type] = true
    connectionStatus.value[type] = null
    
    // 获取当前表单配置
    const config = type === 'gitee' ? giteeConfig.value : githubConfig.value
    
    // 验证必填字段
    if (!config.repo_url || !config.username) {
      connectionStatus.value[type] = {
        title: '连接失败',
        type: 'error',
        message: '请填写仓库地址和用户名'
      }
      ElMessage.error('请填写仓库地址和用户名')
      return
    }
    
    if (type === 'gitee' && !config.password) {
      connectionStatus.value[type] = {
        title: '连接失败',
        type: 'error',
        message: '请填写Gitee密码'
      }
      ElMessage.error('请填写Gitee密码')
      return
    }
    
    if (type === 'github' && !config.token) {
      connectionStatus.value[type] = {
        title: '连接失败',
        type: 'error',
        message: '请填写GitHub访问令牌'
      }
      ElMessage.error('请填写GitHub访问令牌')
      return
    }
    
    // 构建测试请求数据
    const testData = {
      type: type,
      repo_url: config.repo_url,
      username: config.username,
      email: config.email || ''
    }
    
    if (type === 'gitee') {
      testData.password = config.password
    } else {
      testData.token = config.token
    }
    
    const response = await systemAPI.testGitConnection(testData)
    
    if (response.status === 'success') {
      connectionStatus.value[type] = {
        title: '连接成功',
        type: 'success',
        message: response.message || '连接测试通过，配置正确'
      }
    } else {
      connectionStatus.value[type] = {
        title: '连接失败',
        type: 'error',
        message: response.message || '连接测试失败，请检查配置'
      }
    }
  } catch (error) {
    console.error(`测试${type}连接失败:`, error)
    connectionStatus.value[type] = {
      title: '连接失败',
      type: 'error',
      message: error.response?.data?.message || error.message || '连接测试失败'
    }
  } finally {
    testingConnection.value[type] = false
  }
}

const testGitOperations = async (type) => {
  try {
    testingGitOperations.value[type] = true

    // 获取当前表单配置
    const config = type === 'gitee' ? giteeConfig.value : githubConfig.value

    // 验证必填字段
    if (!config.repo_url || !config.username || !config.email || !config.branch) {
      ElMessage.error('请填写完整的配置信息')
      return
    }

    if (type === 'gitee' && !config.password) {
      ElMessage.error('请填写Gitee密码')
      return
    }

    if (type === 'github' && !config.token) {
      ElMessage.error('请填写GitHub访问令牌')
      return
    }

    // 只为GitHub提供测试功能
    if (type !== 'github') {
      ElMessage.warning('目前仅支持GitHub仓库的代码拉取和提交测试')
      return
    }

    ElMessage.info('正在测试GitHub代码操作，请稍候...')

    // 构建测试请求数据
    const testData = {
      repo_url: config.repo_url,
      username: config.username,
      token: config.token,
      email: config.email,
      branch: config.branch
    }

    // 调用后端API进行测试
    const response = await systemAPI.testGitOperations(testData)

    if (response.success) {
      testResultData.value = response.data
      testResultVisible.value = true

      if (response.data.pull_success && response.data.commit_success && response.data.test_images_txt) {
        ElMessage.success('GitHub代码操作测试全部成功！')
      } else {
        ElMessage.warning('GitHub代码操作测试部分完成，请查看详细结果')
      }
    } else {
      ElMessage.error('测试失败：' + (response.message || '未知错误'))
    }
  } catch (error) {
    console.error('测试Git操作失败:', error)
    const errorMessage = error.response?.data?.message || error.message || '测试失败'
    ElMessage.error('测试失败：' + errorMessage)

    // 即使失败也显示结果，如果有的话
    if (error.response?.data?.data) {
      testResultData.value = error.response.data.data
      testResultVisible.value = true
    }
  } finally {
    testingGitOperations.value[type] = false
  }
}

const handleTestResultClose = () => {
  testResultVisible.value = false
  testResultData.value = null
}

const handleViewCommit = (commitSha) => {
  // 构建GitHub提交URL
  const config = githubConfig.value
  if (config && config.repo_url) {
    // 从仓库URL中提取owner和repo名称
    const repoUrl = config.repo_url.replace('.git', '')
    const match = repoUrl.match(/github\.com\/(.+)\/(.+)/)
    if (match) {
      const owner = match[1]
      const repo = match[2]
      const commitUrl = `https://github.com/${owner}/${repo}/commit/${commitSha}`
      window.open(commitUrl, '_blank')
      return
    }
  }

  ElMessage.warning('无法生成提交链接，请手动查看GitHub仓库')
}

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
.git-config-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

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
  justify-content: space-between;
  font-weight: 600;
  color: #303133;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
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

.connection-status {
  margin-top: 16px;
}

.config-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background-color: #f0f9ff;
  border: 1px solid #b3d8ff;
  border-radius: 6px;
}

.status-text {
  color: #409eff;
  font-size: 13px;
}
</style>