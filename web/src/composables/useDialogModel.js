import { ref, watch } from 'vue'

// el-dialog 的 v-model 透传：本地 visible 与 props.modelValue 双向同步，
// 替代各对话框重复手写的「watch props.modelValue + watch visible」成对监听。
// onOpen 在对话框每次打开时触发，用于重置表单或回填编辑数据。
export function useDialogModel(props, emit, onOpen) {
  const visible = ref(props.modelValue)

  watch(() => props.modelValue, (val) => {
    visible.value = val
    if (val && onOpen) onOpen()
  })

  watch(visible, (val) => {
    emit('update:modelValue', val)
  })

  return visible
}
