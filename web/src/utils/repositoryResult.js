/** 将名称列表格式化为每行一个 */
export const formatNameLines = (names) => (names?.length ? names.join('\n') : '')

const buildSection = (label, names) => {
  if (!names?.length) return null
  return `${label} (${names.length}):\n${formatNameLines(names)}`
}

const buildOtherAcrHintSection = (hints = []) => {
  if (!hints.length) return null
  const lines = hints.map(
    (item) => `「${item.repository_name}」在 ACR「${item.namespace}」中存在，请切换 ACR 后添加`
  )
  return `其他 ACR 提示 (${hints.length}):\n${lines.join('\n')}`
}

/** 批量添加镜像结果：成功 / 重复 / 失败 */
export const buildBatchAddResultText = (data = {}) => {
  const sections = []

  const successNames = data.created_names || []
  const duplicateNames = [
    ...(data.already_exist_names || []),
    ...(data.duplicate_in_input || []),
  ]
  const failedNames = [
    ...(data.missing_in_acr || []),
    ...(data.check_failed_names || []),
  ]

  const successSection = buildSection('成功', successNames)
  const duplicateSection = buildSection('重复', duplicateNames)
  const failedSection = buildSection('失败', failedNames)
  const otherAcrSection = buildOtherAcrHintSection(data.found_in_other_acr || [])

  if (successSection) sections.push(successSection)
  if (duplicateSection) sections.push(duplicateSection)
  if (failedSection) sections.push(failedSection)
  if (otherAcrSection) sections.push(otherAcrSection)

  return sections.join('\n\n')
}

/** 清理无效镜像结果（无清理项时也会返回提示文案） */
export const buildCleanInvalidResultText = (data = {}) => {
  const cleaned = data.cleaned_names || []
  const failed = data.check_failed_names || []
  const sections = []

  if (cleaned.length) {
    sections.push(`清理 (${cleaned.length}):\n${formatNameLines(cleaned)}`)
  } else {
    sections.push('清理 (0):\n未发现无效镜像')
  }

  const failedSection = buildSection('检查失败（未清理）', failed)
  if (failedSection) sections.push(failedSection)

  return sections.join('\n\n')
}
