import request from './request'

export const workflowApi = {
  listTopics: () => request.get<never, { topics: string[] }>('/workflows/topics'),
}
