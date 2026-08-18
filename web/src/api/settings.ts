import { http } from './client'

export interface LLMProfile {
  id: string
  name: string
  base_url: string
  api_key_set?: boolean
  api_key_masked?: string
  model: string
  temperature: number
  max_tokens: number
  timeout_sec: number
  context_window: number
  enabled: boolean
}

export interface LLMSettings {
  profiles: LLMProfile[]
  default_id: string
}

export interface LLMProfileInput {
  id: string
  name: string
  base_url: string
  api_key?: string
  model: string
  temperature: number
  max_tokens: number
  timeout_sec: number
  context_window: number
  enabled: boolean
}

export interface LLMUpdate {
  profiles: LLMProfileInput[]
  default_id: string
}

export type NotifierType = 'wecom' | 'feishu' | 'dingtalk'

export interface NotifierChannel {
  type: NotifierType
  webhook_url: string
  secret_set?: boolean
  enabled: boolean
}

export interface NotifierChannelInput {
  type: NotifierType
  webhook_url: string
  secret: string
  enabled: boolean
}

export interface ModelItem {
  id: string
  name?: string
}

export interface DimensionSpec {
  key: string
  label: string
  description?: string
  weight: number
}

export interface ReviewLimits {
  window_days: number
  max_files: number
  timeout_sec: number
}

export const settingsApi = {
  getLLM: () => http.get<LLMSettings>('/api/admin/settings/llm').then(r => r.data),
  updateLLM: (v: LLMUpdate) => http.put('/api/admin/settings/llm', v).then(r => r.data),
  fetchModels: (base_url: string, api_key?: string) =>
    http.post<{ models: ModelItem[] }>('/api/admin/settings/llm/fetch-models', { base_url, api_key }).then(r => r.data.models),
  getServer: () => http.get<{ base_url: string }>('/api/admin/settings/server').then(r => r.data),
  updateServer: (v: { base_url: string }) => http.put('/api/admin/settings/server', v).then(r => r.data),
  changePassword: (v: { old_password: string; new_password: string }) =>
    http.post('/api/admin/change-password', v).then(r => r.data),
  getNotifiers: () =>
    http.get<{ items: NotifierChannel[] }>('/api/admin/settings/notifications').then(r => r.data),
  updateNotifiers: (v: NotifierChannelInput[]) =>
    http.put('/api/admin/settings/notifications', v).then(r => r.data),
  getDimensions: () =>
    http.get<{ dimensions: DimensionSpec[] }>('/api/admin/settings/dimensions').then(r => r.data.dimensions),
  updateDimensions: (dimensions: DimensionSpec[]) =>
    http.put<{ dimensions: DimensionSpec[] }>('/api/admin/settings/dimensions', { dimensions }).then(r => r.data),
  getReviewLimits: () =>
    http.get<{ limits: ReviewLimits }>('/api/admin/settings/review-limits').then(r => r.data.limits),
  updateReviewLimits: (limits: ReviewLimits) =>
    http.put<{ limits: ReviewLimits }>('/api/admin/settings/review-limits', limits).then(r => r.data.limits),
}
