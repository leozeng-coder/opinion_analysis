import request from './request'
import type { DashboardOverview } from '@/types'

export const dashboardApi = {
  overview: () => request.get<never, DashboardOverview>('/dashboard'),
}
