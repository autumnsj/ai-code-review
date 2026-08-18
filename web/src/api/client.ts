import axios from 'axios'
import { message } from 'antd'

export const http = axios.create({ baseURL: '/' })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

http.interceptors.response.use(
  (resp) => resp,
  (error) => {
    const status = error.response?.status
    if (status === 401) {
      localStorage.removeItem('token')
      if (!location.pathname.startsWith('/login') && !location.pathname.startsWith('/reports/') && !location.pathname.startsWith('/author-reports/') && !location.pathname.startsWith('/setup')) {
        location.href = '/login'
      }
    } else if (status === 503 && error.response?.data?.setup_required) {
      if (!location.pathname.startsWith('/setup')) location.href = '/setup'
    } else {
      message.error(error.response?.data?.error || error.message)
    }
    return Promise.reject(error)
  },
)
