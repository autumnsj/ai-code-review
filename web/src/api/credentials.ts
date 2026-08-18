import { http } from './client'

export type CredentialType = 'ssh' | 'https_token'

export type Provider = 'github' | 'gitlab' | 'gitee' | 'gitea'

export interface Credential {
  id: number
  name: string
  type: CredentialType
  public_key?: string
  fingerprint?: string
  provider?: string
  api_base_url?: string
  secret_set?: boolean
  secret_masked?: string
  created_at: string
}

export interface CredentialWithKey extends Credential {
  private_key?: string // 仅创建自动生成 SSH 密钥时一次性返回
}

export const credentialApi = {
  list: () => http.get<{ items: Credential[] }>('/api/admin/credentials').then(r => r.data.items),
  create: (body: { name: string; type: CredentialType; secret?: string; provider?: string; api_base_url?: string }) =>
    http.post<CredentialWithKey>('/api/admin/credentials', body).then(r => r.data),
  update: (id: number, body: { name?: string; secret?: string; provider?: string; api_base_url?: string }) =>
    http.patch<Credential>(`/api/admin/credentials/${id}`, body).then(r => r.data),
  remove: (id: number) => http.delete(`/api/admin/credentials/${id}`).then(r => r.data),
}
