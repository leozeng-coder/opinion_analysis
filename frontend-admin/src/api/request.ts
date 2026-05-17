import axios from 'axios'
import { message } from 'antd'
import { useAuthStore } from '@/store/auth'

const request = axios.create({
  baseURL: '/api',
  timeout: 20000,
})

request.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  (res) => {
    const data = res.data
    if (data.code !== 0) {
      void message.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message))
    }
    return data.data
  },
  (err) => {
    if (err.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    } else if (err.response?.status === 403) {
      void message.error('权限不足，需要 admin 角色')
    } else {
      void message.error(err.response?.data?.message || '网络错误')
    }
    return Promise.reject(err)
  },
)

export default request
