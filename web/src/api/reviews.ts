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
  author?: string
}

export interface DimensionScore {
  score: number
  label: string
  rationale?: string
}

export interface ReviewLog {
  id: number
  review_id: number
  level: string // info | warn | error
  message: string
  created_at: string
}

export interface Review {
  id: number
  repo_id: number
  repo_name: string
  public_token: string
  commit_sha: string
  base_sha?: string
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
  score_arch?: number
  score_quality?: number
  score_security?: number
  score_maint?: number
  score_dimensions?: Record<string, DimensionScore>
  stats: string
  diff_truncated: boolean
  error?: string
  tokens_used: number
  triggered_at: string
  started_at?: string
  finished_at?: string
}

export interface AuthorReport {
  id: number
  review_id: number
  author: string
  author_name: string
  public_token: string
  repo_name?: string
  commit_sha?: string
  base_sha?: string
  target_ref?: string
  summary?: string
  score_total: number
  score_arch: number
  score_quality: number
  score_security: number
  score_maint: number
  score_dimensions?: Record<string, DimensionScore>
  findings_count: number
  critical_count: number
  high_count: number
  medium_count: number
  low_count: number
  info_count: number
  additions: number
  deletions: number
  files_changed: number
  triggered_at: string
  finished_at?: string
  stats?: string // 所属 review 的 stats JSON 字符串（提交时间区间/收窄/抽样）
}

// dimensionRows 从 review 中提取维度评分行：优先用 score_dimensions，
// 否则回退到旧的四维列（兼容老报告）。
export function dimensionRows(review: Review): { key: string; label: string; score: number }[] {
  if (review.score_dimensions && Object.keys(review.score_dimensions).length > 0) {
    return Object.entries(review.score_dimensions)
      .map(([key, d]) => ({ key, label: d.label || key, score: d.score }))
      .sort((a, b) => a.key.localeCompare(b.key))
  }
  return [
    { key: 'architecture', label: '架构', score: review.score_arch ?? 0 },
    { key: 'quality', label: '质量', score: review.score_quality ?? 0 },
    { key: 'security', label: '安全', score: review.score_security ?? 0 },
    { key: 'maintainability', label: '可维护性', score: review.score_maint ?? 0 },
  ]
}

export const reviewsApi = {
  list: (params: { repo_id?: number; page?: number; page_size?: number } = {}) =>
    http.get<{ items: Review[]; total: number }>('/api/admin/reviews', { params }).then(r => r.data),
  get: (id: number) => http.get<Review>(`/api/admin/reviews/${id}`).then(r => r.data),
  findings: (id: number) =>
    http.get<{ items: Finding[] }>(`/api/admin/reviews/${id}/findings`).then(r => r.data.items),
  authorReports: (id: number) =>
    http
      .get<{ items: AuthorReport[] }>(`/api/admin/reviews/${id}/author-reports`)
      .then(r => r.data.items),
  logs: (id: number, since = 0) =>
    http
      .get<{ items: ReviewLog[]; next_id: number }>(`/api/admin/reviews/${id}/log`, {
        params: { since, limit: 500 },
      })
      .then(r => r.data),
  publicGet: (token: string) =>
    http.get<{ review: Review; findings: Finding[] }>(`/public/reviews/${token}`).then(r => r.data),
  publicAuthorGet: (token: string) =>
    http
      .get<{ report: AuthorReport; findings: Finding[] }>(`/public/author-reports/${token}`)
      .then(r => r.data),
}
