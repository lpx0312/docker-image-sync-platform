<template>
  <el-dialog
    title="修改密码"
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    width="420px"
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="80px"
      @keyup.enter="handleSubmit"
    >
      <el-form-item label="原密码" prop="oldPassword">
        <el-input
          v-model="form.oldPassword"
          type="password"
          show-password
          placeholder="请输入原密码"
        />
      </el-form-item>
      <el-form-item label="新密码" prop="newPassword">
        <el-input
          v-model="form.newPassword"
          type="password"
          show-password
          placeholder="大小写字母+数字+特殊字符，至少8位"
        />
        <div v-if="form.newPassword" class="password-strength">
          <div class="strength-bar">
            <div
              class="strength-fill"
              :style="{ width: passwordStrength.percent + '%', background: passwordStrength.color }"
            />
          </div>
          <span :style="{ color: passwordStrength.color, fontSize: '12px' }">{{ passwordStrength.label }}</span>
        </div>
      </el-form-item>
      <el-form-item label="确认密码" prop="confirmPassword">
        <el-input
          v-model="form.confirmPassword"
          type="password"
          show-password
          placeholder="请再次输入新密码"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" :loading="loading" @click="handleSubmit">确认修改</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, computed, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const props = defineProps({ visible: Boolean })
const emit = defineEmits(['update:visible'])

const router = useRouter()
const authStore = useAuthStore()

const formRef = ref()
const loading = ref(false)
const form = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })

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

const passwordStrength = computed(() => getPasswordStrength(form.value.newPassword))

const validateConfirm = (rule, value, callback) => {
  if (value !== form.value.newPassword) {
    callback(new Error('两次输入的密码不一致'))
  } else {
    callback()
  }
}

const validateNewPassword = (rule, value, callback) => {
  if (value === form.value.oldPassword) {
    callback(new Error('新密码不能与原密码相同'))
  } else {
    const err = validateStrongPassword(value)
    if (err) {
      callback(new Error(err))
    } else {
      callback()
    }
  }
}

const rules = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { validator: validateNewPassword, trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    { validator: validateConfirm, trigger: 'blur' },
  ],
}

watch(() => props.visible, (val) => {
  if (val) {
    form.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
    nextTick(() => {
      formRef.value?.clearValidate()
    })
  }
})

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    ElMessage.warning('密码不符合要求，请检查后重试')
    return
  }

  loading.value = true
  try {
    await authStore.changePassword(form.value.oldPassword, form.value.newPassword)
    ElMessage.success('密码修改成功，请重新登录')
    emit('update:visible', false)
    router.push('/login')
  } catch {
    // 错误已在 API 拦截器中处理
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
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
