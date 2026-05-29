<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑 ACR 配置' : '添加 ACR 配置'"
    width="500px"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="100px"
    >
      <el-form-item label="镜像仓库地址" prop="registry_url">
        <el-input
          v-model="form.registry_url"
          placeholder="registry.cn-hangzhou.aliyuncs.com"
        />
      </el-form-item>

      <el-form-item label="命名空间" prop="namespace">
        <el-input
          v-model="form.namespace"
          placeholder="your-namespace"
        />
      </el-form-item>

      <el-form-item label="用户名" prop="username">
        <el-input
          v-model="form.username"
          placeholder="阿里云用户名"
        />
      </el-form-item>

      <el-form-item label="密码" prop="password">
        <el-input
          v-model="form.password"
          type="password"
          placeholder="阿里云密码"
          show-password
        />
      </el-form-item>

      <el-form-item label="认证服务器">
        <div style="display: flex; align-items: center; gap: 8px; width: 100%;">
          <el-input
            v-model="form.auth_server"
            placeholder="留空自动推断 (如 dockerauth.cn-hangzhou.aliyuncs.com)"
            style="flex: 1;"
          />
          <el-tooltip
            placement="top"
            :width="420"
            trigger="hover"
          >
            <template #content>
              <div style="font-size: 12px; line-height: 1.6;">
                <p style="margin: 0 0 8px 0; font-weight: bold;">如何获取认证服务器地址？</p>
                <p style="margin: 0 0 4px 0;">执行以下命令查看 Www-Authenticate 响应头：</p>
                <code style="display: block; background: #1a1a2e; color: #e94560; padding: 8px; border-radius: 4px; font-size: 11px; white-space: pre-wrap; margin: 4px 0;">curl -I https://&lt;你的registry地址&gt;/v2/</code>
                <p style="margin: 4px 0 0 0;">从 <code>realm="https://<b>dockerauth.cn-hangzhou.aliyuncs.com</b>/auth"</code> 中提取。</p>
              </div>
            </template>
            <el-icon style="cursor: pointer; color: #909399;"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>
      </el-form-item>

      <el-form-item label="Docker Service">
        <div style="display: flex; align-items: center; gap: 8px; width: 100%;">
          <el-input
            v-model="form.docker_service"
            placeholder="留空使用默认值 (registry.aliyuncs.com:cn-hangzhou:26842)"
            style="flex: 1;"
          />
          <el-tooltip
            placement="top"
            :width="420"
            trigger="hover"
          >
            <template #content>
              <div style="font-size: 12px; line-height: 1.6;">
                <p style="margin: 0 0 8px 0; font-weight: bold;">如何获取 Docker Service 值？</p>
                <p style="margin: 0 0 4px 0;">执行以下命令查看 Www-Authenticate 响应头：</p>
                <code style="display: block; background: #1a1a2e; color: #e94560; padding: 8px; border-radius: 4px; font-size: 11px; white-space: pre-wrap; margin: 4px 0;">curl -I https://&lt;你的registry地址&gt;/v2/</code>
                <p style="margin: 4px 0 0 0;">从 <code>service="<b>registry.aliyuncs.com:cn-hangzhou:26842</b>"</code> 中提取。</p>
              </div>
            </template>
            <el-icon style="cursor: pointer; color: #909399;"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">
          确定
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import { acrRegistryAPI } from '@/api'

const props = defineProps({
  modelValue: Boolean,
  editData: Object,
})

const emit = defineEmits(['update:modelValue', 'success'])

const visible = ref(false)
const formRef = ref(null)
const submitting = ref(false)

const isEdit = ref(false)

const form = reactive({
  registry_url: '',
  namespace: '',
  username: '',
  password: '',
  auth_server: '',
  docker_service: '',
})

const rules = {
  registry_url: [{ required: true, message: '请输入镜像仓库地址', trigger: 'blur' }],
  namespace: [{ required: true, message: '请输入命名空间', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.editData) {
    isEdit.value = true
    Object.assign(form, {
      registry_url: props.editData.registry_url,
      namespace: props.editData.namespace,
      username: props.editData.username,
      password: '***',
      auth_server: props.editData.auth_server || '',
      docker_service: props.editData.docker_service || '',
    })
  } else {
    isEdit.value = false
    Object.assign(form, {
      registry_url: '',
      namespace: '',
      username: '',
      password: '',
      auth_server: '',
      docker_service: '',
    })
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const handleClose = () => {
  visible.value = false
  formRef.value?.resetFields()
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true

    if (isEdit.value && props.editData) {
      await acrRegistryAPI.update(props.editData.id, form)
      ElMessage.success('更新成功')
    } else {
      await acrRegistryAPI.create(form)
      ElMessage.success('添加成功')
    }

    emit('success')
    handleClose()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
