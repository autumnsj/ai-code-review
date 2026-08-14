import { useEffect, useState, type ReactNode } from 'react'
import { Spin } from 'antd'
import { setupApi } from '../api/setup'

// RequireBootstrap 在进入管理后台前确认系统已初始化。
// 引导模式下 /api/setup/status 返回 200{initialized:false}；正式模式该路由不存在（404），视为已初始化。
export default function RequireBootstrap({ children }: { children: ReactNode }) {
  const [state, setState] = useState<'loading' | 'ready' | 'setup'>('loading')

  useEffect(() => {
    setupApi.status()
      .then(s => setState(s.initialized ? 'ready' : 'setup'))
      .catch(() => setState('ready')) // 正式模式无该路由 → 404
  }, [])

  useEffect(() => {
    if (state === 'setup' && location.pathname !== '/setup') {
      location.href = '/setup'
    }
  }, [state])

  if (state === 'loading') {
    return <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Spin size="large" /></div>
  }
  if (state === 'setup') return null
  return <>{children}</>
}
