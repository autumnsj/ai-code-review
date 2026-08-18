import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Spin } from 'antd'
import dayjs from 'dayjs'
import { reviewsApi, ReviewLog } from '../../api/reviews'

// 按行内容给一个 CSS class，控制 [stage]/[warn]/error 等高亮。
function lineClass(line: string, level: string): string {
  if (level === 'error' || line.includes('[pi-agent] Error')) return 'log-line log-error'
  if (level === 'warn' || line.startsWith('[warn]')) return 'log-line log-warn'
  if (line.startsWith('[stage]')) return 'log-line log-stage'
  if (line.startsWith('[review]')) return 'log-line log-review'
  return 'log-line'
}

// ReviewLogPanel 实时轮询并展示审查执行日志。running/pending 时每 2 秒增量拉取，
// 结束后停止并保留历史快照；用户向上滚动时暂停自动滚到底部。
export default function ReviewLogPanel({ reviewId, running }: { reviewId: number; running: boolean }) {
  const [logs, setLogs] = useState<ReviewLog[]>([])
  const [since, setSince] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const stickToBottom = useRef(true)

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['review-logs', reviewId, since],
    queryFn: () => reviewsApi.logs(reviewId, since),
    refetchInterval: running ? 2000 : false,
    // 初次 since=0 时拿全量；拿到后用 next_id 推进。
  })

  // 审查从运行中转为结束时补拉一次，确保「完成/失败」收尾行不丢。
  const wasRunning = useRef(running)
  useEffect(() => {
    if (wasRunning.current && !running) {
      refetch()
    }
    wasRunning.current = running
  }, [running, refetch])

  useEffect(() => {
    if (!data) return
    const items = data.items ?? []
    if (items.length === 0) return
    setLogs(prev => {
      const seen = new Set(prev.map(l => l.id))
      return [...prev, ...items.filter(l => !seen.has(l.id))]
    })
    if ((data.next_id ?? 0) > since) setSince(data.next_id ?? 0)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  // 自动滚动：仅当用户停在底部附近时跟随。
  useEffect(() => {
    const el = containerRef.current
    if (!el || !stickToBottom.current) return
    el.scrollTop = el.scrollHeight
  }, [logs])

  const onScroll = () => {
    const el = containerRef.current
    if (!el) return
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    stickToBottom.current = nearBottom
  }

  return (
    <Card
      size="small"
      title={
        <span>
          审查日志
          {running && (
            <span style={{ marginLeft: 12, fontSize: 12, color: '#8c8c8c' }}>
              <Spin size="small" /> 实时输出中…
            </span>
          )}
        </span>
      }
      styles={{ body: { padding: 0 } }}
    >
      <div
        ref={containerRef}
        onScroll={onScroll}
        className="review-log-terminal"
      >
        {logs.length === 0 && (
          <div className="log-line log-empty">
            {isFetching ? '正在加载日志…' : running ? '等待日志输出…' : '本次审查没有产生日志。'}
          </div>
        )}
        {logs.map(l => (
          <div key={l.id} className={lineClass(l.message, l.level)}>
            <span className="log-ts">{dayjs(l.created_at).format('HH:mm:ss')}</span>
            <span className="log-msg">{l.message}</span>
          </div>
        ))}
      </div>
      <style>{`
        .review-log-terminal {
          background: #1e1e2e;
          color: #cdd6f4;
          font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
          font-size: 12.5px;
          line-height: 1.6;
          padding: 12px 14px;
          max-height: 360px;
          overflow-y: auto;
          border-radius: 0 0 8px 8px;
        }
        .review-log-terminal .log-line { white-space: pre-wrap; word-break: break-all; }
        .review-log-terminal .log-ts { color: #6c7086; margin-right: 10px; user-select: none; }
        .review-log-terminal .log-stage { color: #89b4fa; font-weight: 600; }
        .review-log-terminal .log-review { color: #a6e3a1; }
        .review-log-terminal .log-warn { color: #f9e2af; }
        .review-log-terminal .log-error { color: #f38ba8; }
        .review-log-terminal .log-empty { color: #6c7086; }
      `}</style>
    </Card>
  )
}
