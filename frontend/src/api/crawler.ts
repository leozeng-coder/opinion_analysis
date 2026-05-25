import axios, { AxiosInstance } from 'axios'
import { message } from 'antd'

// 创建自定义类型，拦截器返回的是 data 而不是 AxiosResponse
interface CrawlerAxiosInstance extends AxiosInstance {
  get<T = any>(url: string, config?: any): Promise<T>
  post<T = any>(url: string, data?: any, config?: any): Promise<T>
}

// 为 MediaCrawler API 创建独立的 axios 实例（不使用标准的响应拦截器）
const crawlerRequest = axios.create({
  baseURL: '/api',
  timeout: 15000,
}) as CrawlerAxiosInstance

// 只添加错误处理，不处理响应格式转换
crawlerRequest.interceptors.response.use(
  (res) => res.data,
  (err) => {
    message.error(err.response?.data?.detail || err.response?.data?.message || '网络错误')
    return Promise.reject(err)
  }
)

// ==================== 类型定义 ====================

export interface MediaCrawlerStartRequest {
  platform: 'xhs' | 'dy' | 'ks' | 'bili' | 'wb' | 'tieba' | 'zhihu'
  login_type: 'qrcode' | 'cookie'
  crawler_type: 'search' | 'detail' | 'creator'
  keywords?: string
  specified_ids?: string
  creator_ids?: string
  save_option: 'db' | 'json' | 'jsonl' | 'csv' | 'sqlite' | 'mongodb' | 'excel'
  enable_comments: boolean
  enable_sub_comments: boolean
  headless: boolean
  start_page: number
  cookies?: string
}

export interface CrawlerStatus {
  status: 'idle' | 'running' | 'stopping' | 'error'
  platform?: string
  crawler_type?: string
  started_at?: string
  error_message?: string
}

export interface CrawlerLog {
  id: number
  timestamp: string
  level: 'info' | 'warning' | 'error' | 'success' | 'debug'
  message: string
}

export interface Platform {
  value: string
  label: string
  icon: string
}

export interface ConfigOption {
  value: string
  label: string
}

export interface ConfigOptions {
  login_types: ConfigOption[]
  crawler_types: ConfigOption[]
  save_options: ConfigOption[]
}

// ==================== API 接口 ====================

export const crawlerApi = {
  /**
   * 启动爬虫
   */
  start: (config: MediaCrawlerStartRequest): Promise<{ status: string; message: string }> =>
    crawlerRequest.post('/crawler/start', config),

  /**
   * 停止爬虫
   */
  stop: (): Promise<{ status: string; message: string }> =>
    crawlerRequest.post('/crawler/stop'),

  /**
   * 获取爬虫状态
   */
  getStatus: (): Promise<CrawlerStatus> =>
    crawlerRequest.get('/crawler/status'),

  /**
   * 获取日志
   */
  getLogs: (limit = 100): Promise<{ logs: CrawlerLog[] }> =>
    crawlerRequest.get('/crawler/logs', {
      params: { limit },
    }),

  /**
   * 获取支持的平台列表
   */
  getPlatforms: (): Promise<{ platforms: Platform[] }> =>
    crawlerRequest.get('/crawler/platforms'),

  /**
   * 获取配置选项
   */
  getOptions: (): Promise<ConfigOptions> =>
    crawlerRequest.get('/crawler/options'),
}
