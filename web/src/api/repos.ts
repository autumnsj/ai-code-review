import { http } from './client'

export interface Repo {
  id: number
  provider: string
  clone_url: string
  web_url: string
  name: string
  default_branch: string
  hook_url: string
  has_secret: boolean
  status: string
}

export const reposApi = {
  list: () => http.get<{ items: Repo[] }>('/api/admin/repos').then(r => r.data.items),
  create: (v: Partial<Repo> & { clone_url: string; name: string }) =>
    http.post<Repo>('/api/admin/repos', v).then(r => r.data),
  get: (id: number) => http.get<Repo>(`/api/admin/repos/${id}`).then(r => r.data),
  update: (id: number, v: Partial<Repo>) =>
    http.patch<Repo>(`/api/admin/repos/${id}`, v).then(r => r.data),
  remove: (id: number) => http.delete(`/api/admin/repos/${id}`).then(r => r.data),
  resetToken: (id: number) =>
    http.post<{ hook_url: string }>(`/api/admin/repos/${id}/reset-token`).then(r => r.data),
}
