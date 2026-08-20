<template>
  <el-dialog
    v-model="visible"
    :title="title"
    :width="width"
    class="result-report-dialog"
    @close="handleDialogClose"
  >
    <div class="report-summary" :class="tone">
      <el-icon class="summary-icon"><component :is="summaryIcon" /></el-icon>
      <span class="summary-text">{{ summary || defaultSummary }}</span>
    </div>

    <div class="report-body">
      <div v-for="(section, si) in visibleSections" :key="si" class="report-section">
        <div class="section-header">
          <span class="section-dot" :class="section.tone || 'info'"></span>
          <span class="section-label">{{ section.label }}</span>
          <span class="section-count">{{ countOf(section) }} 个</span>
        </div>
        <div class="section-items">
          <template v-for="(item, ii) in section.items" :key="ii">
            <span
              v-if="typeof item === 'string'"
              class="item-tag"
              :class="section.tone || 'info'"
            >{{ item }}</span>
            <div v-else class="item-entry">
              <span class="item-title">{{ item.title }}</span>
              <span
                v-for="(tag, ti) in item.tags || []"
                :key="ti"
                class="item-tag"
                :class="tag.tone || 'primary'"
              >{{ tag.label }}</span>
            </div>
          </template>
        </div>
      </div>

      <div v-if="!visibleSections.length" class="report-empty">
        {{ emptyText || '没有可展示的内容' }}
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button v-if="cancelText" @click="handleCancel">{{ cancelText }}</el-button>
        <el-button :type="confirmType" @click="handleConfirm">{{ confirmText || '知道了' }}</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { CircleCheckFilled, WarningFilled, InfoFilled, CircleCloseFilled } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: Boolean,
  title: { type: String, default: '' },
  // 顶部摘要条色调：success | warning | error | info
  tone: { type: String, default: 'info' },
  summary: { type: String, default: '' },
  /**
   * 分区块：[{ label, tone: success|warning|danger|info, items }]
   * items 元素为字符串时渲染成标签块；为 { title, tags: [{label, tone}] }
   * 时渲染成「标题 + 若干标签块」一行（如跨仓库重复：仓库名 + 各仓库别名标签）
   */
  sections: { type: Array, default: () => [] },
  confirmText: { type: String, default: '知道了' },
  cancelText: { type: String, default: '' },
  confirmType: { type: String, default: 'primary' },
  emptyText: { type: String, default: '' },
  width: { type: String, default: '520px' },
})

const emit = defineEmits(['update:modelValue', 'confirm', 'cancel'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})

const visibleSections = computed(() =>
  (props.sections || []).filter((s) => Array.isArray(s.items) && s.items.length > 0)
)

const summaryIcon = computed(() => {
  switch (props.tone) {
    case 'success': return CircleCheckFilled
    case 'warning': return WarningFilled
    case 'error': return CircleCloseFilled
    default: return InfoFilled
  }
})

const defaultSummary = computed(() => {
  const total = visibleSections.value.reduce((sum, s) => sum + (s.items || []).length, 0)
  if (!total) return ''
  const labeled = visibleSections.value.map((s) => `${s.label} ${countOf(s)} 个`).join('，')
  return labeled || `共 ${total} 项`
})

const countOf = (section) => (section.items || []).length

// 关闭动作来源：按钮点击时先记录，待 el-dialog 的 close 事件统一分发，
// 保证 confirm/cancel 只触发一次且不会互相覆盖
let closingAction = null

const handleConfirm = () => {
  closingAction = 'confirm'
  visible.value = false
}

const handleCancel = () => {
  closingAction = 'cancel'
  visible.value = false
}

const handleDialogClose = () => {
  const action = closingAction || 'cancel'
  closingAction = null
  emit(action)
}
</script>

<style scoped>
.report-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 14px;
  margin-bottom: 14px;
}

.report-summary.success { background: #f0f9eb; color: #67c23a; }
.report-summary.warning { background: #fdf6ec; color: #e6a23c; }
.report-summary.error   { background: #fef0f0; color: #f56c6c; }
.report-summary.info    { background: #f4f4f5; color: #909399; }

.summary-icon { font-size: 18px; }
.summary-text { color: #303133; line-height: 1.5; }

.report-body {
  max-height: 420px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.report-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #303133;
  font-weight: 600;
}

.section-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.section-dot.success { background: #67c23a; }
.section-dot.warning { background: #e6a23c; }
.section-dot.danger  { background: #f56c6c; }
.section-dot.info    { background: #909399; }

.section-count {
  font-size: 12px;
  color: #909399;
  font-weight: 400;
}

.section-items {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.item-tag {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
}

.item-tag.success { background: #f0f9eb; color: #67c23a; border: 1px solid #e1f3d8; }
.item-tag.warning { background: #fdf6ec; color: #e6a23c; border: 1px solid #faecd8; }
.item-tag.danger  { background: #fef0f0; color: #f56c6c; border: 1px solid #fde2e2; }
.item-tag.info    { background: #f4f4f5; color: #909399; border: 1px solid #e9e9eb; }
.item-tag.primary { background: #ecf5ff; color: #409eff; border: 1px solid #d9ecff; }

.item-entry {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.item-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.report-empty {
  text-align: center;
  color: #909399;
  padding: 24px 0;
  font-size: 14px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
