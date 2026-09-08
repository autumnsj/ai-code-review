import { http } from './client'

// Report 一条已发送的日报/周报（reports 表）。列表接口不返回 content。
export interface Report {
  id: number
  kind: string // daily | weekly
  trigger_type: string // scheduled | manual
  period_start: string
  period_end: string
  title: string
  content?: string
  job_id: number
  created_at: string
}

export const reportsApi = {
  list: (params: { page?: number; page_size?: number } = {}) =>
    http.get<{ items: Report[]; total: number }>('/api/admin/reports', { params }).then(r => r.data),
  get: (id: number) => http.get<Report>(`/api/admin/reports/${id}`).then(r => r.data),
}
