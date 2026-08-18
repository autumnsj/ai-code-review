import { http } from './client'

export interface Repo {
  id: number
  provider: string
  clone_url: string
  web_url: string
  name: string
  default_branch: string
  credential_id?: number
  credential_name?: string
  hook_url: string
  has_secret: boolean
  status: string
}

export interface RepoUpdate {
  name?: string
  default_branch?: string
  access_token?: string
  credential_id?: number | null
  hook_secret?: string
  status?: string
}

export interface ImportPreviewRepo {
  name: string
  clone_url: string
  web_url: string
  default_branch: string
  private: boolean
  already_imported: boolean
}

export interface ImportCommitItem {
  name: string
  clone_url: string
  web_url: string
  default_branch: string
}

export interface ImportResultRepo {
  id: number
  name: string
  hook_url: string
  action: 'created' | 'updated'
  default_branch?: string
  default_branch_changed?: boolean
  hook_registered: boolean
  hook_error?: string
}

export const reposApi = {
  list: () => http.get<{ items: Repo[] }>('/api/admin/repos').then(r => r.data.items),
  create: (v: Partial<Repo> & { clone_url: string; name: string; access_token?: string; hook_secret?: string; credential_id?: number }) =>
    http.post<Repo>('/api/admin/repos', v).then(r => r.data),
  get: (id: number) => http.get<Repo>(`/api/admin/repos/${id}`).then(r => r.data),
  update: (id: number, v: RepoUpdate) =>
    http.patch<Repo>(`/api/admin/repos/${id}`, v).then(r => r.data),
  remove: (id: number) => http.delete(`/api/admin/repos/${id}`).then(r => r.data),
  resetToken: (id: number) =>
    http.post<{ hook_url: string }>(`/api/admin/repos/${id}/reset-token`).then(r => r.data),
  registerWebhook: (id: number) =>
    http.post<{ created: boolean; already_exists: boolean; hook_id?: string; hook_url: string }>(`/api/admin/repos/${id}/webhook`).then(r => r.data),
  registerAllWebhooks: () =>
    http.post<{
      total: number
      created: number
      existed: number
      skipped: number
      failed: number
      items: {
        repo_id: number
        repo_name: string
        created: boolean
        already_exists: boolean
        hook_id?: string
        hook_url?: string
        default_branch?: string
        default_branch_changed?: boolean
        skipped?: string
        error?: string
      }[]
    }>('/api/admin/webhooks/register-all').then(r => r.data),
  trigger: (
    id: number,
    body: { mode?: 'commit' | 'branch' | 'repo'; commit_sha?: string; base_sha?: string; target_ref?: string; source_ref?: string; ref?: string; force?: boolean },
  ) => http.post<{ review_id: number; public_token: string }>(`/api/admin/repos/${id}/trigger`, body).then(r => r.data),
  importPreview: (body: { provider: string; api_base_url?: string; credential_id: number }) =>
    http.post<{ login: string; repos: ImportPreviewRepo[] }>('/api/admin/import/preview', body).then(r => r.data),
  importCommit: (body: {
    provider: string
    api_base_url?: string
    credential_id: number
    hook_secret?: string
    items: ImportCommitItem[]
  }) => http.post<{ results: ImportResultRepo[]; created: number; updated: number; failed: number }>('/api/admin/import/commit', body).then(r => r.data),
}
