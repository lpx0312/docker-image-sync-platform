<template>
  <div class="aliyun-config-container">
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Monitor /></el-icon>
          <span class="header-title">阿里云镜像仓库配置</span>
          <div class="header-actions">
            <el-button type="primary" size="small" @click="showAddDialog">
              添加新 ACR
            </el-button>
          </div>
        </div>
      </template>

      <div class="config-section">
        <!-- 默认 ACR 选择 -->
        <div class="default-acr-section">
          <el-form-item label="默认 ACR">
            <el-select
              v-model="defaultAcrId"
              placeholder="选择默认 ACR"
              @change="handleDefaultChange"
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
        </div>

        <!-- ACR 列表 -->
        <el-table :data="acrList" style="width: 100%; margin-top: 20px">
          <el-table-column type="index" label="序号" width="60" />
          <el-table-column prop="registry_url" label="镜像仓库地址" />
          <el-table-column prop="namespace" label="命名空间" />
          <el-table-column prop="username" label="用户名" />
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.is_default" type="success" size="small">默认</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="showEditDialog(row)">
                编辑
              </el-button>
              <el-popconfirm
                title="确定要删除这个 ACR 配置吗？"
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
import { ElMessage } from 'element-plus'
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

const loadAcrList = async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    if (response.data && response.data.status === 'success') {
      acrList.value = response.data.data || []
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        defaultAcrId.value = defaultAcr.id
      }
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
    ElMessage.error('加载 ACR 列表失败')
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
    ElMessage.success('默认 ACR 已更新')
    await loadAcrList()
  } catch (error) {
    ElMessage.error('设置默认 ACR 失败')
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
