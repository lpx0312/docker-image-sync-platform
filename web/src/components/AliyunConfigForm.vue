<template>
  <div class="aliyun-config-container">
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Monitor /></el-icon>
          <span class="header-title">镜像仓库配置</span>
          <div class="header-actions">
            <el-button type="primary" size="small" @click="showAddDialog">
              添加镜像仓库
            </el-button>
          </div>
        </div>
      </template>

      <div class="config-section">
        <!-- 默认 ACR 选择 -->
        <div class="default-acr-section">
          <el-form-item label="默认仓库">
            <el-select
              v-model="defaultAcrId"
              placeholder="选择默认镜像仓库"
              @change="handleDefaultChange"
              style="width: 100%"
            >
              <el-option
                v-for="item in acrList"
                :key="item.id"
                :label="`${item.alias || item.namespace}（${typeLabel(item)}）`"
                :value="item.id"
              />
            </el-select>
          </el-form-item>
        </div>

        <!-- ACR 列表 -->
        <el-table :data="acrList" style="width: 100%; margin-top: 20px">
          <el-table-column type="index" label="序号" width="60" />
          <el-table-column prop="registry_url" label="镜像仓库地址" />
          <el-table-column prop="alias" label="别名">
            <template #default="{ row }">
              {{ row.alias || row.namespace }}
            </template>
          </el-table-column>
          <el-table-column prop="namespace" label="命名空间" />
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <el-tag :type="row.registry_type === 'swr' ? 'warning' : 'primary'" size="small">
                {{ typeLabel(row) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="username" label="用户名" />
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.is_default" type="success" size="small">默认</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="210">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="showEditDialog(row)">
                编辑
              </el-button>
              <el-button
                type="success"
                link
                size="small"
                :loading="testingId === row.id"
                @click="handleTest(row)"
              >
                测试
              </el-button>
              <el-popconfirm
                title="确定要删除这个镜像仓库配置吗？"
                @confirm="handleDelete(row)"
              >
                <template #reference>
                  <el-button type="danger" link size="small">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- ACR 配置对话框 -->
    <AcrRegistryDialog
      v-model="dialogVisible"
      :edit-data="editData"
      @success="loadAcrList"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Monitor } from '@element-plus/icons-vue'
import { acrRegistryAPI } from '@/api'
import AcrRegistryDialog from './AcrRegistryDialog.vue'

const acrList = ref([])
const defaultAcrId = ref(null)
const dialogVisible = ref(false)
const editData = ref(null)

onMounted(() => {
  loadAcrList()
})

const typeLabel = (row) => (row.registry_type === 'swr' ? '华为 SWR' : row.registry_type === 'ccr' ? '腾讯 CCR' : '阿里 ACR')

const testingId = ref(null)

const handleTest = async (row) => {
  testingId.value = row.id
  try {
    const response = await acrRegistryAPI.test(row.id)
    const r = response?.data || {}
    const lines = []
    lines.push(r.login_ok ? `✓ 登录凭证：${r.login_message}` : `✗ 登录凭证：${r.login_message}`)
    if (r.registry_type === 'swr') {
      if (r.manage_skipped) {
        lines.push(`– AK/SK：${r.manage_message}`)
      } else {
        lines.push(r.manage_ok ? `✓ AK/SK：${r.manage_message}` : `✗ AK/SK：${r.manage_message}`)
      }
    }
    const ok = r.login_ok && (r.registry_type !== 'swr' || r.manage_ok || r.manage_skipped)
    ElMessageBox.alert(lines.join('\n'), `连接测试 - ${row.alias || row.namespace}`, {
      type: ok ? 'success' : 'error',
      confirmButtonText: '知道了',
    })
  } catch (error) {
    ElMessage.error('测试失败: ' + (error.response?.data?.message || error.message || '未知错误'))
  } finally {
    testingId.value = null
  }
}

const loadAcrList = async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    // axios 拦截器已返回 response.data，所以 response 就是 {status: "success", data: [...]}
    if (response && response.status === 'success') {
      acrList.value = response.data || []
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        defaultAcrId.value = defaultAcr.id
      }
    }
  } catch (error) {
    console.error('加载镜像仓库列表失败:', error)
    ElMessage.error('加载镜像仓库列表失败')
  }
}

const showAddDialog = () => {
  editData.value = null
  dialogVisible.value = true
}

const showEditDialog = (row) => {
  editData.value = { ...row }
  dialogVisible.value = true
}

const handleDefaultChange = async (id) => {
  try {
    await acrRegistryAPI.setDefault(id)
    ElMessage.success('默认镜像仓库已更新')
    await loadAcrList()
  } catch (error) {
    ElMessage.error('设置默认镜像仓库失败')
  }
}

const handleDelete = async (row) => {
  try {
    await acrRegistryAPI.delete(row.id)
    ElMessage.success('删除成功')
    await loadAcrList()
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || '未知错误'))
  }
}
</script>

<style scoped>
.aliyun-config-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.config-card {
  border-radius: 12px;
  border: 1px solid #e4e7ed;
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

.default-acr-section {
  max-width: 400px;
}
</style>
