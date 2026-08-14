import axios from 'axios'

// 引导期使用独立的 axios 实例，不带 token，避免全局 401 拦截干扰。
const http = axios.create({ baseURL: '/' })

export interface SetupStatus {
  initialized: boolean
  sqlite_path: string
  data_dir: string
}

export interface TestResult {
  ok: boolean
  error?: string
}

export const setupApi = {
  status: () => http.get<SetupStatus>('/api/setup/status').then(r => r.data),
  test: (body: { driver: string; dsn: string }) =>
    http.post<TestResult>('/api/setup/test', body).then(r => r.data),
  complete: (body: { driver: string; dsn: string; admin_password: string; base_url: string }) =>
    http.post<{ ok: boolean }>('/api/setup/complete', body).then(r => r.data),
}
