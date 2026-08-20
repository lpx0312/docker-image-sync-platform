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

  <!-- 统一结果报告弹窗（与批量添加一致） -->
  <ResultReportDialog
    v-model="report.visible"
    :title="report.title"
    :tone="report.tone"
    :summary="report.summary"
    :sections="report.sections"
    :confirm-text="report.confirmText"
    :cancel-text="report.cancelText"
    :confirm-type="report.confirmType"
    :empty-text="report.emptyText"
    :width="report.width"
    @confirm="confirmReport"
    @cancel="cancelReport"
  />
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { acrRepositoryAPI } from '@/api'
import ResultReportDialog from '@/components/ResultReportDialog.vue'
import { useResultReport } from '@/composables/useResultReport'

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

const { report, openReport, confirmReport, cancelReport } = useResultReport()

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true

    const response = await acrRepositoryAPI.create({
      acr_registry_id: props.acrRegistryId,
      repository_name: form.repository_name,
    })
    const result = response?.data || {}

    if (result.created) {
      ElMessage.success('添加成功')
      emit('success')
      handleClose()
    } else {
      // 与批量添加一致的分区提示；未创建成功时保留表单便于修改
      const name = form.repository_name
      const sectionMap = {
        already_exist: { label: '本地已存在，未重复添加', tone: 'info' },
        missing_in_registry: { label: '目标仓库中不存在，已跳过', tone: 'warning' },
        check_failed: { label: '检查目标仓库失败，未添加', tone: 'danger' },
      }
      const conf = sectionMap[result.reason] || { label: '未添加', tone: 'warning' }
      openReport({
        title: '添加镜像结果',
        tone: conf.tone === 'info' ? 'info' : 'warning',
        sections: [{ ...conf, items: [name] }],
      })
    }
  } catch (error) {
    if (error !== false) {
      ElMessage.error('添加失败: ' + (error.response?.data?.message || error.message || '未知错误'))
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
