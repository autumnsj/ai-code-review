import { http } from './client'

export interface DashboardRecent {
  id: number
  repo_name: string
  status: string
  score_total: number
  commit_sha: string
  target_ref: string
  finished_at?: string | null
}

export interface Dashboard {
  repo_count: number
  review_count: number
  succeeded: number
  failed: number
  pending: number
  avg_score: number
  recent_reviews: DashboardRecent[]
}

export interface Job {
  id: number
  kind: string
  status: string
  attempts: number
  max_attempts: number
  last_error: string
  idempotency_key: string
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

export const opsApi = {
  dashboard: () => http.get<Dashboard>('/api/admin/dashboard').then(r => r.data),
  listJobs: (params: { status?: string; page?: number; page_size?: number }) =>
    http.get<{ items: Job[]; total: number; page: number; page_size: number }>('/api/admin/jobs', { params }).then(r => r.data),
  retryJob: (id: number) => http.post(`/api/admin/jobs/${id}/retry`).then(r => r.data),
}
