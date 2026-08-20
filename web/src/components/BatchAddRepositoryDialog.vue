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

  <!-- 统一结果报告弹窗 -->
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
    @update:model-value="val => !val && cancelReport()"
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
  repository_names: '',
})

const rules = {
  repository_names: [{ required: true, message: '请输入镜像列表', trigger: 'blur' }],
}

const { report, openReport, confirmReport, cancelReport } = useResultReport()

const showBatchAddResult = (response) => {
  const data = response?.data ?? {}

  const sections = [
    { label: '成功添加', tone: 'success', items: data.created_names || [] },
    { label: '本地已存在，未重复添加', tone: 'info', items: data.already_exist_names || [] },
    { label: '输入重复，已忽略', tone: 'warning', items: data.duplicate_in_input || [] },
    { label: '目标仓库中不存在，已跳过', tone: 'warning', items: data.missing_in_acr || [] },
    { label: '检查失败，已跳过', tone: 'danger', items: data.check_failed_names || [] },
  ].filter(s => s.items.length > 0)

  if (!sections.length) {
    ElMessage.info('没有可添加的新镜像')
    return
  }

  const hasIssue = !!(
    data.already_exist_names?.length
    || data.duplicate_in_input?.length
    || data.missing_in_acr?.length
    || data.check_failed_names?.length
  )

  openReport({
    title: '批量添加结果',
    tone: hasIssue ? 'warning' : 'success',
    sections,
  })
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
