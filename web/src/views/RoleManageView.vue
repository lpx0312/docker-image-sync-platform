<template>
  <div class="role-manage-container">
    <div class="tab-toolbar">
      <el-button type="primary" @click="openCreateDialog">
        <el-icon><Plus /></el-icon>新建角色
      </el-button>
      <el-button @click="loadRoles">
        <el-icon><Refresh /></el-icon>刷新
      </el-button>
    </div>

    <el-table :data="roles" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="name" label="角色名称" width="140" />
      <el-table-column prop="code" label="标识" width="140" />
      <el-table-column label="类型" width="110">
        <template #default="{ row }">
          <el-tag :type="row.is_system ? 'warning' : 'info'" size="small">
            {{ row.is_system ? '系统内置' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="权限" min-width="280">
        <template #default="{ row }">
          <el-tag
            v-for="perm in row.permissions || []"
            :key="perm"
            size="small"
            style="margin-right: 6px; margin-bottom: 4px;"
          >{{ permissionLabel(perm) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="user_count" label="用户数" width="90" />
      <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip />
      <el-table-column label="操作" width="160" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
          <el-popconfirm
            title="确定删除该角色？"
            :disabled="row.is_system || row.user_count > 0"
            @confirm="handleDelete(row.id)"
          >
            <template #reference>
              <el-button
                size="small"
                type="danger"
                :disabled="row.is_system || row.user_count > 0"
              >删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      :title="dialogMode === 'create' ? '新建角色' : '编辑角色'"
      v-model="showDialog"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="角色名称" prop="name">
          <el-input v-model="form.name" placeholder="如：审计员" />
        </el-form-item>
        <el-form-item label="角色标识" prop="code">
          <el-input
            v-model="form.code"
            placeholder="小写字母开头，如 auditor"
            :disabled="dialogMode === 'edit'"
          />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
        <el-form-item label="权限" prop="permissions">
          <el-checkbox-group v-model="form.permissions">
            <el-checkbox
              v-for="perm in allPermissions"
              :key="perm.code"
              :label="perm.code"
              :disabled="isPermissionLocked(perm.code)"
            >{{ perm.name }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { roleAPI } from '@/api'
import { permissionLabel } from '@/constants/permissions'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

const roles = ref([])
const allPermissions = ref([])
const loading = ref(false)
const showDialog = ref(false)
const dialogMode = ref('create')
const submitLoading = ref(false)
const editingId = ref(null)
const formRef = ref()
const form = ref({
  name: '',
  code: '',
  description: '',
  permissions: [],
})

const rules = {
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
  code: [{ required: true, message: '请输入角色标识', trigger: 'blur' }],
  permissions: [{ type: 'array', required: true, min: 1, message: '至少选择一项权限', trigger: 'change' }],
}

function isPermissionLocked(code) {
  return dialogMode.value === 'edit' && form.value.code === 'admin' && (code === 'users' || code === 'roles')
}

async function loadRoles() {
  loading.value = true
  try {
    const res = await roleAPI.listRoles()
    roles.value = res.data || []
  } catch {
    //
  } finally {
    loading.value = false
  }
}

async function loadPermissions() {
  try {
    const res = await roleAPI.listPermissions()
    allPermissions.value = res.data || []
  } catch {
    //
  }
}

function openCreateDialog() {
  dialogMode.value = 'create'
  editingId.value = null
  form.value = { name: '', code: '', description: '', permissions: ['sync'] }
  showDialog.value = true
}

function openEditDialog(row) {
  dialogMode.value = 'edit'
  editingId.value = row.id
  form.value = {
    name: row.name,
    code: row.code,
    description: row.description || '',
    permissions: [...(row.permissions || [])],
  }
  showDialog.value = true
}

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitLoading.value = true
  try {
    if (dialogMode.value === 'create') {
      await roleAPI.createRole(form.value)
      ElMessage.success('角色创建成功')
    } else {
      await roleAPI.updateRole(editingId.value, {
        name: form.value.name,
        description: form.value.description,
        permissions: form.value.permissions,
      })
      ElMessage.success('角色已更新')
    }
    showDialog.value = false
    loadRoles()
  } catch {
    //
  } finally {
    submitLoading.value = false
  }
}

async function handleDelete(id) {
  try {
    await roleAPI.deleteRole(id)
    ElMessage.success('角色已删除')
    loadRoles()
  } catch {
    //
  }
}

onMounted(() => {
  loadPermissions()
  loadRoles()
})
</script>

<style scoped>
.role-manage-container {
  max-width: var(--max-width);
  margin: 0 auto;
  background: var(--color-bg-card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
  box-shadow: var(--shadow-sm);
  padding: var(--space-lg);
}

.tab-toolbar {
  display: flex;
  gap: var(--space-sm);
  margin-bottom: var(--space-md);
}
</style>
