<template>
  <el-dialog
    v-model="visible"
    title="添加镜像"
    width="500px"
    @close="handleClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="镜像名称" prop="repository_name">
        <el-input
          v-model="form.repository_name"
          placeholder="例如: nginx、library/mysql"
        />
      </el-form-item>
      <el-form-item>
        <el-text type="info" size="small">
          输入镜像名称，不含 tag 和 registry 地址
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
import { ElMessage } from 'element-plus'
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
  repository_name: '',
})

const rules = {
  repository_name: [{ required: true, message: '请输入镜像名称', trigger: 'blur' }],
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val) {
    form.repository_name = ''
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

    await acrRepositoryAPI.create({
      acr_registry_id: props.acrRegistryId,
      repository_name: form.repository_name,
    })

    ElMessage.success('添加成功')
    emit('success')
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
