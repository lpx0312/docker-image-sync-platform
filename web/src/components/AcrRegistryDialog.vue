<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑镜像仓库配置' : '添加镜像仓库配置'"
    width="520px"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="100px"
    >
      <el-form-item label="仓库类型" prop="registry_type">
        <el-radio-group v-model="form.registry_type">
          <el-radio value="acr">阿里云 ACR</el-radio>
          <el-radio value="swr">华为云 SWR</el-radio>
        </el-radio-group>
      </el-form-item>

      <el-form-item label="镜像仓库地址" prop="registry_url">
        <el-input
          v-model="form.registry_url"
          :placeholder="isSwr ? 'swr.cn-east-4.myhuaweicloud.com' : 'registry.cn-hangzhou.aliyuncs.com'"
        />
      </el-form-item>

      <el-form-item :label="isSwr ? '组织' : '命名空间'" prop="namespace">
        <el-input
          v-model="form.namespace"
          :placeholder="isSwr ? 'SWR 组织名（需预先在华为云控制台创建）' : 'your-namespace'"
        />
      </el-form-item>

      <el-form-item label="别名" prop="alias">
        <el-input
          v-model="form.alias"
          placeholder="平台内展示与选择用的唯一标识（ACR/SWR 命名空间可能同名）"
        />
      </el-form-item>

      <el-form-item label="用户名" prop="username">
        <el-input
          v-model="form.username"
          :placeholder="isSwr ? '格式：区域@AK，如 cn-east-4@HPUAXXXXXXXX' : '阿里云用户名'"
        />
      </el-form-item>

      <el-form-item label="密码" prop="password">
        <el-input
          v-model="form.password"
          type="password"
          :placeholder="isSwr ? '华为云 SK（与登录用户名配对的密钥）' : '阿里云密码'"
          show-password
        />
      </el-form-item>

      <template v-if="isSwr">
        <el-divider content-position="left">管理面凭证（可选，用于获取镜像列表）</el-divider>
        <el-form-item label="Access Key">
          <el-input
            v-model="form.access_key"
            placeholder="IAM 访问密钥 AK（华为云控制台「我的凭证 → 访问密钥」）"
          />
        </el-form-item>
        <el-form-item label="Secret Key">
          <el-input
            v-model="form.secret_key"
            type="password"
            placeholder="与 AK 配对的 SK；编辑时留空表示不修改"
            show-password
          />
        </el-form-item>
        <el-alert
          type="info"
          :closable="false"
          show-icon
          style="margin-bottom: 18px;"
        >
          <template #title>
            上方「用户名/密码」即 docker login 凭证（区域@AK / 登录密码），用于推送、拉取与 Tag 查询；
            AK/SK 仅在「从仓库导入」获取镜像列表时使用（SWR 管理面 API）。
            组织（namespace）需预先在华为云控制台创建。
          </template>
        </el-alert>
      </template>

      <template v-if="!isSwr">
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
      </template>
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
import { ref, reactive, computed, watch } from 'vue'
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
  alias: '',
  username: '',
  password: '',
  auth_server: '',
  docker_service: '',
  registry_type: 'acr',
  access_key: '',
  secret_key: '',
})

const isSwr = computed(() => form.registry_type === 'swr')

// 新增时别名默认跟随命名空间预填，手动改过则不再跟随
const aliasManuallySet = ref(false)
watch(() => form.namespace, (val) => {
  if (!isEdit.value && !aliasManuallySet.value) {
    form.alias = val
  }
})
watch(() => form.alias, (val) => {
  if (!isEdit.value && val && val !== form.namespace) {
    aliasManuallySet.value = true
  }
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
      alias: props.editData.alias || props.editData.namespace || '',
      username: props.editData.username,
      password: '***',
      auth_server: props.editData.auth_server || '',
      docker_service: props.editData.docker_service || '',
      registry_type: props.editData.registry_type || 'acr',
      access_key: props.editData.access_key || '',
      secret_key: '',
    })
  } else {
    isEdit.value = false
    Object.assign(form, {
      registry_url: '',
      namespace: '',
      alias: '',
      username: '',
      password: '',
      auth_server: '',
      docker_service: '',
      registry_type: 'acr',
      access_key: '',
      secret_key: '',
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
