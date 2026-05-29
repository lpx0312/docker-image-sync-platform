export const PERM_SYNC = 'sync'
export const PERM_IMAGES = 'images'
export const PERM_GITHUB = 'github'
export const PERM_CONFIG = 'config'
export const PERM_USERS = 'users'
export const PERM_ROLES = 'roles'

export const PERMISSION_LABELS = {
  [PERM_SYNC]: '镜像同步',
  [PERM_IMAGES]: '镜像管理',
  [PERM_GITHUB]: 'GitHub Actions',
  [PERM_CONFIG]: '系统配置',
  [PERM_USERS]: '用户管理',
  [PERM_ROLES]: '角色管理',
}

export function permissionLabel(code) {
  return PERMISSION_LABELS[code] || code
}
