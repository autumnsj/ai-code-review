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
}
