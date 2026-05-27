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
    })
  } else {
    isEdit.value = false
    Object.assign(form, {
      registry_url: '',
      namespace: '',
      username: '',
      password: '',
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
