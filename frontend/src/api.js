import axios from 'axios'
import { useSession } from './session'

export const http = axios.create({ baseURL: '/api/v1' })

http.interceptors.request.use((cfg) => {
  const session = useSession()
  if (session.accessToken) {
    cfg.headers.Authorization = `Bearer ${session.accessToken}`
  }
  return cfg
})

let refreshing = null
http.interceptors.response.use(
  (res) => res,
  async (err) => {
    const session = useSession()
    const url = err.config?.url || ''
    if (err.response?.status === 401 && session.refreshToken && !err.config?._retry && !url.includes('/auth/')) {
      err.config._retry = true
      refreshing =
        refreshing ||
        axios
          .post('/api/v1/auth/refresh', { refreshToken: session.refreshToken })
          .then((res) => {
            session.apply(res.data.data)
            return session.accessToken
          })
          .finally(() => {
            refreshing = null
          })
      try {
        const token = await refreshing
        err.config.headers.Authorization = `Bearer ${token}`
        return http(err.config)
      } catch (refreshErr) {
        session.clear()
        throw refreshErr
      }
    }
    return Promise.reject(err)
  },
)

export async function post(path, body = {}, headers = {}) {
  const { data } = await http.post(path, body, { headers })
  return data
}

export function errorText(err) {
  return err?.response?.data?.msg || err?.message || '请求失败'
}
