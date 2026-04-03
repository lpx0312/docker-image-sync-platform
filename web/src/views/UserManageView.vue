<template>
  <div class="user-manage-container">
    <el-tabs v-model="activeTab">
      <!-- 用户列表 -->
      <el-tab-pane label="用户列表" name="users">
        <div class="tab-toolbar">
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>新建用户
          </el-button>
          <el-button @click="loadUsers">
            <el-icon><Refresh /></el-icon>刷新
          </el-button>
        </div>

        <el-table :data="users" v-loading="usersLoading" stripe>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="username" label="用户名" width="150" />
          <el-table-column prop="email" label="邮箱" min-width="180" />
          <el-table-column prop="role" label="角色" width="120">
            <template #default="{ row }">
              <el-tag :type="roleTagType(row.role)" size="small">
                {{ roleLabel(row.role) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
                {{ row.status === 'active' ? '正常' : '已禁用' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="last_login_at" label="最后登录" width="180">
            <template #default="{ row }">
              {{ row.last_login_at ? formatTime(row.last_login_at) : '从未登录' }}
            </template>
          </el-table-column>
          <el-table-column label="操作" width="320" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="openChangeRole(row)">角色</el-button>
              <el-button
                size="small"
                :type="row.status === 'active' ? 'warning' : 'success'"
                @click="toggleUserStatus(row)"
              >
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button size="small" @click="openResetPassword(row)">重置密码</el-button>
              <el-popconfirm
                title="确定删除该用户？"
                @confirm="handleDeleteUser(row.id)"
              >
                <template #reference>
                  <el-button size="small" type="danger">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          v-if="usersTotal > 0"
          class="pagination"
          :current-page="usersPage"
          :page-size="usersPageSize"
          :total="usersTotal"
          layout="total, prev, pager, next"
          @current-change="(p) => { usersPage = p; loadUsers() }"
        />
      </el-tab-pane>

      <!-- 登录日志 -->
      <el-tab-pane label="登录日志" name="logs">
        <div class="tab-toolbar">
          <el-input
            v-model="logSearch"
            placeholder="搜索用户名"
            clearable
            style="width: 200px"
            @clear="loadLogs"
            @keyup.enter="loadLogs"
          />
          <el-button @click="loadLogs">
            <el-icon><Search /></el-icon>搜索
          </el-button>
        </div>

        <el-table :data="logs" v-loading="logsLoading" stripe>
          <el-table-column prop="username" label="用户名" width="120" />
          <el-table-column prop="ip" label="IP地址" width="140" />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">
                {{ row.status === 'success' ? '成功' : '失败' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="说明" min-width="160" />
          <el-table-column prop="user_agent" label="User-Agent" min-width="200" show-overflow-tooltip />
          <el-table-column prop="created_at" label="时间" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>

        <el-pagination
          v-if="logsTotal > 0"
          class="pagination"
          :current-page="logsPage"
          :page-size="logsPageSize"
          :total="logsTotal"
          layout="total, prev, pager, next"
          @current-change="(p) => { logsPage = p; loadLogs() }"
        />
      </el-tab-pane>
    </el-tabs>

    <!-- 新建用户对话框 -->
    <el-dialog title="新建用户" v-model="showCreateDialog" width="450px" :close-on-click-modal="false">
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createForm.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="createForm.password" type="password" show-password placeholder="大小写字母+数字+特殊字符，至少8位" />
          <div v-if="createForm.password" class="password-strength">
            <div class="strength-bar">
              <div
                class="strength-fill"
                :style="{ width: createPasswordStrength.percent + '%', background: createPasswordStrength.color }"
              />
            </div>
            <span :style="{ color: createPasswordStrength.color, fontSize: '12px' }">{{ createPasswordStrength.label }}</span>
          </div>
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="createForm.email" placeholder="可选" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="createForm.role" style="width: 100%">
            <el-option label="普通用户" value="user" />
            <el-option label="运维员" value="operator" />
            <el-option label="管理员" value="admin" />
          </el-select>
        </el-form-item>
        <el-form-item label="权限预览">
          <div class="permission-preview">
            <el-tag
              v-for="perm in getPermissionLabels(createForm.role)"
              :key="perm"
              size="small"
              type="success"
              style="margin-right: 6px; margin-bottom: 4px;"
            >{{ perm }}</el-tag>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" :loading="createLoading" @click="handleCreateUser">创建</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码对话框 -->
    <el-dialog title="重置密码" v-model="showResetDialog" width="400px" :close-on-click-modal="false">
      <p style="margin-bottom: 16px; color: #606266;">
        为用户 <strong>{{ resetTarget?.username }}</strong> 设置新密码：
      </p>
      <el-form ref="resetFormRef" :model="resetForm" :rules="resetRules">
        <el-form-item prop="newPassword">
          <el-input v-model="resetForm.newPassword" type="password" show-password placeholder="大小写字母+数字+特殊字符，至少8位" />
          <div v-if="resetForm.newPassword" class="password-strength">
            <div class="strength-bar">
              <div
                class="strength-fill"
                :style="{ width: resetPasswordStrength.percent + '%', background: resetPasswordStrength.color }"
              />
            </div>
            <span :style="{ color: resetPasswordStrength.color, fontSize: '12px' }">{{ resetPasswordStrength.label }}</span>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showResetDialog = false">取消</el-button>
        <el-button type="primary" :loading="resetLoading" @click="handleResetPassword">确认重置</el-button>
      </template>
    </el-dialog>

    <!-- 修改角色对话框 -->
    <el-dialog title="修改角色" v-model="showRoleDialog" width="450px" :close-on-click-modal="false">
      <p style="margin-bottom: 16px; color: #606266;">
        修改用户 <strong>{{ roleTarget?.username }}</strong> 的角色：
      </p>
      <el-form label-width="80px">
        <el-form-item label="当前角色">
          <el-tag :type="roleTagType(roleTarget?.role)" size="small">
            {{ roleLabel(roleTarget?.role) }}
          </el-tag>
        </el-form-item>
        <el-form-item label="新角色">
          <el-select v-model="newRole" style="width: 100%">
            <el-option label="普通用户" value="user" />
            <el-option label="运维员" value="operator" />
            <el-option label="管理员" value="admin" />
          </el-select>
        </el-form-item>
        <el-form-item label="权限预览">
          <div class="permission-preview">
            <el-tag
              v-for="perm in getPermissionLabels(newRole)"
              :key="perm"
              size="small"
              type="success"
              style="margin-right: 6px; margin-bottom: 4px;"
            >{{ perm }}</el-tag>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRoleDialog = false">取消</el-button>
        <el-button type="primary" :loading="roleLoading" @click="handleUpdateRole">确认修改</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { authAPI } from '@/api'
import { Plus, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

const activeTab = ref('users')

const users = ref([])
const usersLoading = ref(false)
const usersPage = ref(1)
const usersPageSize = ref(20)
const usersTotal = ref(0)

const logs = ref([])
const logsLoading = ref(false)
const logsPage = ref(1)
const logsPageSize = ref(20)
const logsTotal = ref(0)
const logSearch = ref('')

function validateStrongPassword(password) {
  if (password.length < 8) return '密码长度至少8位'
  if (!/[A-Z]/.test(password)) return '密码必须包含至少一个大写字母'
  if (!/[a-z]/.test(password)) return '密码必须包含至少一个小写字母'
  if (!/[0-9]/.test(password)) return '密码必须包含至少一个数字'
  if (!/[^A-Za-z0-9]/.test(password)) return '密码必须包含至少一个特殊字符'
  return null
}

function getPasswordStrength(password) {
  if (!password) return { percent: 0, color: '#dcdfe6', label: '' }
  let score = 0
  if (password.length >= 8) score++
  if (password.length >= 12) score++
  if (/[A-Z]/.test(password)) score++
  if (/[a-z]/.test(password)) score++
  if (/[0-9]/.test(password)) score++
  if (/[^A-Za-z0-9]/.test(password)) score++
  if (score <= 2) return { percent: 25, color: '#f56c6c', label: '弱' }
  if (score <= 4) return { percent: 55, color: '#e6a23c', label: '中' }
  if (score <= 5) return { percent: 80, color: '#409eff', label: '强' }
  return { percent: 100, color: '#67c23a', label: '非常强' }
}

const strongPasswordValidator = (rule, value, callback) => {
  const err = validateStrongPassword(value)
  if (err) {
    callback(new Error(err))
  } else {
    callback()
  }
}

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createFormRef = ref()
const createForm = ref({ username: '', password: '', email: '', role: 'user' })
const createPasswordStrength = computed(() => getPasswordStrength(createForm.value.password))
const createRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { validator: strongPasswordValidator, trigger: 'blur' },
  ],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
}

const showResetDialog = ref(false)
const resetLoading = ref(false)
const resetFormRef = ref()
const resetTarget = ref(null)
const resetForm = ref({ newPassword: '' })
const resetPasswordStrength = computed(() => getPasswordStrength(resetForm.value.newPassword))
const resetRules = {
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { validator: strongPasswordValidator, trigger: 'blur' },
  ],
}

const showRoleDialog = ref(false)
const roleLoading = ref(false)
const roleTarget = ref(null)
const newRole = ref('user')

const permissionMap = {
  sync: '镜像同步',
  github: 'GitHub Actions',
  config: '系统配置',
  users: '用户管理',
}

const rolePermissions = {
  admin: ['sync', 'github', 'config', 'users'],
  operator: ['sync', 'github', 'config'],
  user: ['sync'],
}

function roleLabel(role) {
  const map = { admin: '管理员', operator: '运维员', user: '普通用户' }
  return map[role] || role
}

function roleTagType(role) {
  const map = { admin: 'danger', operator: 'warning', user: 'info' }
  return map[role] || 'info'
}

function getPermissionLabels(role) {
  const perms = rolePermissions[role] || ['sync']
  return perms.map(p => permissionMap[p] || p)
}

function openChangeRole(user) {
  roleTarget.value = user
  newRole.value = user.role
  showRoleDialog.value = true
}

async function handleUpdateRole() {
  if (!roleTarget.value || newRole.value === roleTarget.value.role) {
    showRoleDialog.value = false
    return
  }
  roleLoading.value = true
  try {
    await authAPI.updateUserRole(roleTarget.value.id, { role: newRole.value })
    ElMessage.success('角色已更新')
    showRoleDialog.value = false
    loadUsers()
  } catch {
    //
  } finally {
    roleLoading.value = false
  }
}

function formatTime(t) {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN')
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const res = await authAPI.listUsers({ page: usersPage.value, page_size: usersPageSize.value })
    users.value = res.data || []
    usersTotal.value = res.total || 0
  } catch {
    //
  } finally {
    usersLoading.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const res = await authAPI.getLoginLogs({
      page: logsPage.value,
      page_size: logsPageSize.value,
      username: logSearch.value,
    })
    logs.value = res.data || []
    logsTotal.value = res.total || 0
  } catch {
    //
  } finally {
    logsLoading.value = false
  }
}

async function toggleUserStatus(user) {
  const newStatus = user.status === 'active' ? 'disabled' : 'active'
  try {
    await authAPI.updateUserStatus(user.id, { status: newStatus })
    ElMessage.success('状态已更新')
    loadUsers()
  } catch {
    //
  }
}

async function handleDeleteUser(id) {
  try {
    await authAPI.deleteUser(id)
    ElMessage.success('用户已删除')
    loadUsers()
  } catch {
    //
  }
}

async function handleCreateUser() {
  if (!createFormRef.value) return
  const valid = await createFormRef.value.validate().catch(() => false)
  if (!valid) {
    ElMessage.warning('密码不符合强密码要求，请检查后重试')
    return
  }

  createLoading.value = true
  try {
    await authAPI.createUser(createForm.value)
    ElMessage.success('用户创建成功')
    showCreateDialog.value = false
    createForm.value = { username: '', password: '', email: '', role: 'user' }
    loadUsers()
  } catch {
    //
  } finally {
    createLoading.value = false
  }
}

function openResetPassword(user) {
  resetTarget.value = user
  resetForm.value = { newPassword: '' }
  showResetDialog.value = true
}

async function handleResetPassword() {
  if (!resetFormRef.value) return
  const valid = await resetFormRef.value.validate().catch(() => false)
  if (!valid) {
    ElMessage.warning('密码不符合强密码要求，请检查后重试')
    return
  }

  resetLoading.value = true
  try {
    await authAPI.resetUserPassword(resetTarget.value.id, { new_password: resetForm.value.newPassword })
    ElMessage.success('密码已重置')
    showResetDialog.value = false
  } catch {
    //
  } finally {
    resetLoading.value = false
  }
}

onMounted(() => {
  loadUsers()
})

watch(activeTab, (tab) => {
  if (tab === 'logs' && logs.value.length === 0) {
    loadLogs()
  }
})
</script>

<style scoped>
.user-manage-container {
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

.pagination {
  margin-top: var(--space-md);
  padding-top: var(--space-md);
  border-top: 1px solid var(--color-border-light);
  display: flex;
  justify-content: flex-end;
}

.permission-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.password-strength {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.strength-bar {
  flex: 1;
  height: 4px;
  background: #e4e7ed;
  border-radius: 2px;
  overflow: hidden;
}

.strength-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s, background 0.3s;
}
</style>
