import { Tag } from 'antd'

const MAP: Record<string, { color: string; text: string }> = {
  critical: { color: 'red', text: '严重' },
  high: { color: 'volcano', text: '高危' },
  medium: { color: 'orange', text: '中等' },
  low: { color: 'blue', text: '低危' },
  info: { color: 'default', text: '提示' },
}

export default function SeverityTag({ severity }: { severity: string }) {
  const m = MAP[severity] || MAP.info
  return <Tag color={m.color}>{m.text}</Tag>
}

export function StatusTag({ status }: { status: string }) {
  const m: Record<string, { color: string; text: string }> = {
    pending: { color: 'default', text: '排队中' },
    running: { color: 'processing', text: '审查中' },
    succeeded: { color: 'success', text: '已完成' },
    failed: { color: 'error', text: '失败' },
    dead: { color: 'error', text: '已放弃' },
  }
  const x = m[status] || { color: 'default', text: status }
  return <Tag color={x.color}>{x.text}</Tag>
}

// scoreColor 根据 0-100 评分返回展示颜色。
export function scoreColor(score: number): string {
  if (score >= 85) return '#3f8600'
  if (score >= 70) return '#d48806'
  if (score >= 60) return '#d46b08'
  return '#cf1322'
}
