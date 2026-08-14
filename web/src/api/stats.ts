import { http } from './client'

export interface AuthorSummary {
  author: string
  review_count: number
  avg_total: number
  avg_arch: number
  avg_quality: number
  avg_security: number
  avg_maint: number
  additions: number
  deletions: number
  files_changed: number
  tokens_used: number
  findings_total: number
  critical: number
  high: number
  medium: number
  low: number
  info: number
  last_reviewed: string
}

export interface AuthorDetail {
  summary: AuthorSummary
  recent: Array<{
    id: number
    repo_name: string
    score_total: number
    additions: number
    deletions: number
    finished_at?: string | null
  }>
}

export const statsApi = {
  listAuthors: (params: { days?: number; repo_id?: number; sort?: string; page?: number; page_size?: number }) =>
    http.get<{ items: AuthorSummary[]; page: number; page_size: number }>('/api/admin/stats/authors', { params }).then(r => r.data),
  getAuthor: (author: string, params: { days?: number; repo_id?: number }) =>
    http.get<AuthorDetail>(`/api/admin/stats/authors/${encodeURIComponent(author)}`, { params }).then(r => r.data),
}
