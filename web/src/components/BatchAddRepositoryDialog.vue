<template>
  <el-dialog
    v-model="visible"
    title="批量添加镜像"
    width="600px"
    @close="handleClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="镜像列表" prop="repository_names">
        <el-input
          v-model="form.repository_names"
          type="textarea"
          :rows="10"
          placeholder="每行一个镜像名称，例如:&#10;nginx&#10;redis&#10;mysql"
        />
      </el-form-item>
      <el-form-item>
        <el-text type="info" size="small">
          每行输入一个镜像名称，不含 tag 和 registry 地址
        </el-text>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { acrRepositoryAPI } from '@/api'

const props = defineProps({
  modelValue: Boolean,
  acrRegistryId: Number,
})

const emit = defineEmits(['update:modelValue', 'success'])

const visible = ref(false)
const formRef = ref(null)
const submitting = ref(false)

const form = reactive({
  repository_names: '',
})

const rules = {
  repository_names: [{ required: true, message: '请输入镜像列表', trigger: 'blur' }],
}

const formatNameList = (names) => (names && names.length ? names.join('、') : '无')

const showBatchAddResult = (response) => {
  const data = response?.data ?? {}
  const sections = []

  if (data.created_names?.length) {
    sections.push(`成功添加 ${data.created_names.length} 个：\n${formatNameList(data.created_names)}`)
  }
  if (data.duplicate_in_input?.length) {
    sections.push(`输入列表中重复，已忽略 ${data.duplicate_in_input.length} 个：\n${formatNameList(data.duplicate_in_input)}`)
  }
  if (data.already_exist_names?.length) {
    sections.push(`本地已存在，未重复添加 ${data.already_exist_names.length} 个：\n${formatNameList(data.already_exist_names)}`)
  }
  if (data.missing_in_acr?.length) {
    sections.push(`ACR 中不存在，已跳过 ${data.missing_in_acr.length} 个：\n${formatNameList(data.missing_in_acr)}`)
  }
  if (data.check_failed_names?.length) {
    sections.push(`检查失败，已跳过 ${data.check_failed_names.length} 个：\n${formatNameList(data.check_failed_names)}`)
  }

  const hasIssue = !!(data.duplicate_in_input?.length
    || data.already_exist_names?.length
    || data.missing_in_acr?.length
    || data.check_failed_names?.length)

  if (sections.length > 0) {
    ElMessageBox.alert(sections.join('\n\n'), '批量添加结果', {
      type: hasIssue ? 'warning' : 'success',
      confirmButtonText: '知道了',
    })
    return
  }

  if (response?.message) {
    ElMessageBox.alert(response.message, '批量添加结果', {
      type: hasIssue || /不存在|失败|重复|已存在/.test(response.message)
        ? 'warning'
        : (data.created > 0 ? 'success' : 'info'),
      confirmButtonText: '知道了',
    })
    return
  }

  ElMessage.info('没有可添加的新镜像')
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val) {
    form.repository_names = ''
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

    const names = form.repository_names
      .split('\n')
      .map(n => n.trim())
      .filter(n => n !== '')

    if (names.length === 0) {
      ElMessage.warning('请输入至少一个镜像名称')
      return
    }

    const response = await acrRepositoryAPI.batchCreate({
      acr_registry_id: props.acrRegistryId,
      repository_names: names,
    })

    showBatchAddResult(response)
    if (response.data?.created > 0) {
      emit('success')
    }
    handleClose()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('添加失败: ' + (error.message || '未知错误'))
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
