import { http } from './client'

export interface Member {
  id: number
  git_login: string
  display_name: string
  team: string
  note: string
  active: boolean
  created_at: string
}

export interface MemberInput {
  git_login: string
  display_name?: string
  team?: string
  note?: string
  active?: boolean
}

export interface MemberUpdate {
  display_name?: string
  team?: string
  note?: string
  active?: boolean
}

export const memberApi = {
  list: () => http.get<{ items: Member[] }>('/api/admin/members').then(r => r.data.items),
  create: (body: MemberInput) => http.post<Member>('/api/admin/members', body).then(r => r.data),
  update: (id: number, body: MemberUpdate) =>
    http.patch<Member>(`/api/admin/members/${id}`, body).then(r => r.data),
  remove: (id: number) => http.delete(`/api/admin/members/${id}`).then(r => r.data),
  unknown: () => http.get<{ items: string[] }>('/api/admin/unknown-members').then(r => r.data.items),
}
