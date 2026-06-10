import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import { message } from 'antd'
import { useAuthStore } from '@/store/auth'

interface CustomAxiosInstance extends AxiosInstance {
  get<T = any>(url: string, config?: any): Promise<T>
  post<T = any>(url: string, data?: any, config?: any): Promise<T>
  put<T = any>(url: string, data?: any, config?: any): Promise<T>
  delete<T = any>(url: string, config?: any): Promise<T>
}

const request = axios.create({
  baseURL: '/api',
  timeout: 15000,
}) as CustomAxiosInstance

request.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = useAuthStore.getState().token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  (res: AxiosResponse) => {
    const data = res.data
    if (data.code !== 0) {
      message.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message))
    }
    return data.data
  },
  (err: any) => {
    if (err.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    } else {
      message.error(err.response?.data?.message || '网络错误')
    }
    return Promise.reject(err)
  }
)

export default request
