import { http } from './client'

// Report 一条已发送的日报/周报（reports 表）。列表接口不返回 content。
export interface Report {
  id: number
  kind: string // daily | weekly
  trigger_type: string // scheduled | manual
  author: string // 个人报告的成员名；空串 = 团队简报/异常提醒
  period_start: string
  period_end: string
  title: string
  content?: string
  job_id: number
  created_at: string
}

export const reportsApi = {
  list: (params: { page?: number; page_size?: number; author?: string } = {}) =>
    http.get<{ items: Report[]; total: number }>('/api/admin/reports', { params }).then(r => r.data),
  get: (id: number) => http.get<Report>(`/api/admin/reports/${id}`).then(r => r.data),
}
