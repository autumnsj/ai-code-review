import { http } from './client'

export interface Finding {
  id: number
  source: string
  rule_id: string
  severity: string
  category: string
  file_path: string
  line_start: number
  line_end: number
  title: string
  message: string
  snippet?: string
  suggestion?: string
  confidence?: string
}

export interface Review {
  id: number
  repo_id: number
  repo_name: string
  public_token: string
  commit_sha: string
  target_ref: string
  source_ref: string
  pr_number: number
  pr_title: string
  pr_url: string
  author: string
  event_type: string
  status: string
  summary: string
  score_total: number
  score_arch: number
  score_quality: number
  score_security: number
  score_maint: number
  stats: string
  diff_truncated: boolean
  error?: string
  tokens_used: number
  triggered_at: string
  started_at?: string
  finished_at?: string
}

export const reviewsApi = {
  list: (params: { repo_id?: number; page?: number; page_size?: number } = {}) =>
    http.get<{ items: Review[]; total: number }>('/api/admin/reviews', { params }).then(r => r.data),
  get: (id: number) => http.get<Review>(`/api/admin/reviews/${id}`).then(r => r.data),
  findings: (id: number) =>
    http.get<{ items: Finding[] }>(`/api/admin/reviews/${id}/findings`).then(r => r.data.items),
  publicGet: (token: string) =>
    http.get<{ review: Review; findings: Finding[] }>(`/public/reviews/${token}`).then(r => r.data),
}
