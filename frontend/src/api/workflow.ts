import request from './request'
import { Workflow, WorkflowExecution, WorkflowNodeExecution, PageData } from '@/types'

export const workflowApi = {
  // 列表
  list: (params: { page: number; pageSize: number }) =>
    request.get<PageData<Workflow>>('/workflows', { params }),

  // 创建
  create: (data: Partial<Workflow>) =>
    request.post<Workflow>('/workflows', data),

  // 详情
  detail: (id: number) =>
    request.get<Workflow>(`/workflows/${id}`),

  // 更新
  update: (id: number, data: Partial<Workflow>) =>
    request.put<Workflow>(`/workflows/${id}`, data),

  // 删除
  delete: (id: number) =>
    request.delete(`/workflows/${id}`),

  // 手动执行
  execute: (id: number, input?: Record<string, any>) =>
    request.post<WorkflowExecution>(`/workflows/${id}/execute`, { input }),

  // 执行历史
  executions: (id: number, params: { page: number; pageSize: number }) =>
    request.get<PageData<WorkflowExecution>>(`/workflows/${id}/executions`, { params }),

  // 执行日志
  executionLogs: (execId: number) =>
    request.get<WorkflowNodeExecution[]>(`/workflows/executions/${execId}/logs`),

  // 取消执行
  cancelExecution: (execId: number) =>
    request.post<{ message: string }>(`/workflows/executions/${execId}/cancel`),

  // 获取所有话题列表
  listTopics: () =>
    request.get<{ topics: string[] }>('/workflows/topics'),

  // 获取爬虫配置上限（供工作流编辑器动态限制用）
  getCrawlerLimits: () =>
    request.get<{ maxNotesCount: number; maxCommentsCount: number; maxSubCommentsCount: number }>('/admin/system/crawler/limits'),
}

export const reportApi = {
  // 重新生成报告（涉及多轮 LLM 调用，需要较长超时）
  regenerate: (data: { executionId: number; format?: string; htmlTheme?: string }) =>
    request.post<{ reportId: string; format: string; articleCount: number; downloadUrl: string }>('/reports/regenerate', data, { timeout: 180000 }),
}
