import { useState } from 'react'
import { Card, Col, Empty, Row, Select, Space, Spin, Tag, Tooltip, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { statsApi, AuthorSummary } from '../../api/stats'
import { reposApi, Repo } from '../../api/repos'
import { scoreColor } from '../../components/SeverityTag'

const { Title, Text } = Typography

const DAYS_OPTIONS = [
  { value: 7, label: '近 7 天' },
  { value: 30, label: '近 30 天' },
  { value: 90, label: '近 90 天' },
  { value: 0, label: '全部' },
]

interface MetricDef {
  key: string
  label: string
  // 从作者行取该指标的数值
  value: (a: AuthorSummary) => number
  // 格式化数值展示
  format: (v: number) => string
  // 柱子颜色（取值越大颜色越深/语义色）
  color: string
  // 数值是否为评分（0-100），评分用 scoreColor
  isScore?: boolean
  hint?: string
}

const CODE_METRICS: MetricDef[] = [
  { key: 'churn', label: '总代码量', hint: '新增 + 删除行数',
    value: (a) => a.churn, format: (v) => v.toLocaleString(), color: '#1677ff' },
  { key: 'additions', label: '新增行数',
    value: (a) => a.additions, format: (v) => '+' + v.toLocaleString(), color: '#52c41a' },
  { key: 'deletions', label: '删除行数',
    value: (a) => a.deletions, format: (v) => '-' + v.toLocaleString(), color: '#ff4d4f' },
  { key: 'review_count', label: '审查次数',
    value: (a) => a.review_count, format: (v) => v.toLocaleString(), color: '#722ed1' },
  { key: 'findings_total', label: '问题数量',
    value: (a) => a.findings_total, format: (v) => v.toLocaleString(), color: '#fa8c16' },
]

const SCORE_METRICS: MetricDef[] = [
  { key: 'avg_total', label: '综合评分', value: (a) => a.avg_total, format: (v) => v.toFixed(1), color: '#1677ff', isScore: true },
  { key: 'avg_arch', label: '架构', value: (a) => a.avg_arch, format: (v) => v.toFixed(1), color: '#1677ff', isScore: true },
  { key: 'avg_quality', label: '质量', value: (a) => a.avg_quality, format: (v) => v.toFixed(1), color: '#1677ff', isScore: true },
  { key: 'avg_security', label: '安全', value: (a) => a.avg_security, format: (v) => v.toFixed(1), color: '#1677ff', isScore: true },
  { key: 'avg_maint', label: '可维护', value: (a) => a.avg_maint, format: (v) => v.toFixed(1), color: '#1677ff', isScore: true },
]

function displayName(a: AuthorSummary) {
  return a.display_name || a.author
}

function BarRow({ a, rank, max, metric }: { a: AuthorSummary; rank: number; max: number; metric: MetricDef }) {
  const v = metric.value(a)
  const pct = max > 0 ? Math.max(4, Math.round((v / max) * 100)) : 0
  const color = metric.isScore ? scoreColor(v) : metric.color
  const rankColor = rank === 1 ? '#fadb14' : rank === 2 ? '#d9d9d9' : rank === 3 ? '#d48806' : '#bfbfbf'
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0' }}>
      <div style={{
        width: 22, height: 22, borderRadius: '50%', flex: '0 0 22px',
        background: rankColor, color: rank <= 3 ? '#333' : '#666',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 12, fontWeight: 700,
      }}>{rank}</div>
      <Tooltip title={a.display_name ? `${a.display_name} @${a.author}` : a.author}>
        <div style={{ width: 130, flex: '0 0 130', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: 500 }}>
          {displayName(a)}
          {a.team && <Tag style={{ marginLeft: 4, fontSize: 11 }}>{a.team}</Tag>}
        </div>
      </Tooltip>
      <div style={{ flex: 1, background: '#f5f5f5', borderRadius: 4, height: 18, position: 'relative', overflow: 'hidden' }}>
        <div style={{
          width: `${pct}%`, height: '100%', background: color, opacity: 0.85,
          borderRadius: 4, transition: 'width .3s ease',
        }} />
      </div>
      <div style={{ width: 90, flex: '0 0 90', textAlign: 'right', fontWeight: 600, color }}>
        {metric.format(v)}
      </div>
    </div>
  )
}

function BoardCard({ metric, rows }: { metric: MetricDef; rows: AuthorSummary[] }) {
  const max = rows.length ? Math.max(...rows.map(metric.value)) : 0
  return (
    <Card size="small" title={
      <Space size={6}>
        <span>{metric.label}</span>
        {metric.hint && <Text type="secondary" style={{ fontSize: 12, fontWeight: 400 }}>{metric.hint}</Text>}
      </Space>
    } bodyStyle={{ padding: '10px 14px' }}>
      {rows.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />
      ) : (
        rows.map((a, i) => (
          <BarRow key={a.author} a={a} rank={i + 1} max={max} metric={metric} />
        ))
      )}
    </Card>
  )
}

export default function LeaderboardPage() {
  const [days, setDays] = useState(30)
  const [repoId, setRepoId] = useState<number | undefined>()

  const { data: boards, isLoading } = useQuery({
    queryKey: ['stats-leaderboard', days, repoId],
    queryFn: () => statsApi.leaderboard({ days, repo_id: repoId, limit: 10 }),
  })
  const { data: repos } = useQuery({ queryKey: ['repos'], queryFn: reposApi.list })

  const renderGroup = (title: string, metrics: MetricDef[]) => (
    <>
      <Title level={5} style={{ marginTop: 8, marginBottom: 12 }}>{title}</Title>
      <Row gutter={[16, 16]}>
        {metrics.map((m) => (
          <Col xs={24} sm={12} lg={8} key={m.key}>
            <BoardCard metric={m} rows={boards?.[m.key] ?? []} />
          </Col>
        ))}
      </Row>
    </>
  )

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={3} style={{ margin: 0 }}>排行榜</Title>
        <Space wrap>
          <Select style={{ width: 130 }} value={days} options={DAYS_OPTIONS} onChange={setDays} />
          <Select
            style={{ width: 220 }} allowClear placeholder="全部仓库"
            value={repoId} onChange={setRepoId}
            options={(repos as Repo[] | undefined)?.map(r => ({ value: r.id, label: r.name })) ?? []}
          />
        </Space>
      </div>

      <Spin spinning={isLoading}>
        {boards ? (
          <>
            {renderGroup('代码量', CODE_METRICS)}
            {renderGroup('评分（越高越好）', SCORE_METRICS)}
          </>
        ) : (
          <Card><Empty description="暂无数据" /></Card>
        )}
      </Spin>
    </div>
  )
}
