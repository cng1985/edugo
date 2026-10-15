import { defineStore } from 'pinia'

export const useSession = defineStore('session', {
  state: () => ({
    accessToken: localStorage.getItem('edugo.access') || '',
    refreshToken: localStorage.getItem('edugo.refresh') || '',
    user: JSON.parse(localStorage.getItem('edugo.user') || 'null'),
  }),
  getters: {
    roles: (s) => s.user?.roles || [],
    isAdmin: (s) => (s.user?.roles || []).includes('admin'),
    canTeach: (s) => {
      const roles = s.user?.roles || []
      return roles.includes('admin') || roles.includes('teacher')
    },
  },
  actions: {
    apply(pair) {
      this.accessToken = pair.accessToken
      this.refreshToken = pair.refreshToken
      this.user = pair.user
      localStorage.setItem('edugo.access', pair.accessToken)
      localStorage.setItem('edugo.refresh', pair.refreshToken)
      localStorage.setItem('edugo.user', JSON.stringify(pair.user))
    },
    clear() {
      this.accessToken = ''
      this.refreshToken = ''
      this.user = null
      localStorage.removeItem('edugo.access')
      localStorage.removeItem('edugo.refresh')
      localStorage.removeItem('edugo.user')
    },
  },
})
