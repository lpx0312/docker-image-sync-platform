import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authAPI } from '@/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || sessionStorage.getItem('token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || sessionStorage.getItem('user') || 'null'))
  const lastActivityTime = ref(Date.now())
  let autoLogoutTimer = null

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role_code === 'admin')
  const username = computed(() => user.value?.username || '')
  const permissions = computed(() => user.value?.permissions || [])

  function hasPermission(perm) {
    return permissions.value.includes(perm)
  }

  async function login(username, password, rememberMe = false) {
    const res = await authAPI.login({ username, password, remember_me: rememberMe })
    token.value = res.token
    user.value = res.user

    const storage = rememberMe ? localStorage : sessionStorage
    storage.setItem('token', res.token)
    storage.setItem('user', JSON.stringify(res.user))

    if (!rememberMe) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    } else {
      sessionStorage.removeItem('token')
      sessionStorage.removeItem('user')
    }

    startAutoLogoutTimer()
    return res
  }

  async function logout() {
    if (token.value) {
      try {
        await authAPI.logout()
      } catch {
        // ignore logout API errors
      }
    }
    clearAuth()
  }

  function clearAuth() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    sessionStorage.removeItem('token')
    sessionStorage.removeItem('user')
    stopAutoLogoutTimer()
  }

  async function fetchCurrentUser() {
    if (!token.value) return
    try {
      const res = await authAPI.getCurrentUser()
      user.value = res
      const storage = localStorage.getItem('token') ? localStorage : sessionStorage
      storage.setItem('user', JSON.stringify(res))
    } catch {
      clearAuth()
    }
  }

  async function changePassword(oldPassword, newPassword) {
    await authAPI.changePassword({ old_password: oldPassword, new_password: newPassword })
    clearAuth()
  }

  function updateActivityTime() {
    lastActivityTime.value = Date.now()
  }

  function startAutoLogoutTimer() {
    stopAutoLogoutTimer()
    autoLogoutTimer = setInterval(() => {
      const timeout = 30 * 60 * 1000
      if (Date.now() - lastActivityTime.value > timeout) {
        clearAuth()
        window.location.href = '/login?expired=1'
      }
    }, 60 * 1000)
  }

  function stopAutoLogoutTimer() {
    if (autoLogoutTimer) {
      clearInterval(autoLogoutTimer)
      autoLogoutTimer = null
    }
  }

  if (token.value) {
    startAutoLogoutTimer()
  }

  return {
    token,
    user,
    lastActivityTime,
    isLoggedIn,
    isAdmin,
    username,
    permissions,
    hasPermission,
    login,
    logout,
    clearAuth,
    fetchCurrentUser,
    changePassword,
    updateActivityTime,
    startAutoLogoutTimer,
    stopAutoLogoutTimer,
  }
})
