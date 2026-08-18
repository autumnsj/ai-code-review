import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Layout, Card, Descriptions, Table, Tag, Space, Typography, Empty, Spin, theme,
} from 'antd'
import dayjs from 'dayjs'
import { reviewsApi, Finding, AuthorReport, DimensionScore } from '../../api/reviews'
import ScoreRing from '../../components/ScoreRing'
import SeverityTag from '../../components/SeverityTag'

const { Text } = Typography
const { Header, Content } = Layout

function authorDimensionRows(report: AuthorReport) {
  if (report.score_dimensions && Object.keys(report.score_dimensions).length > 0) {
    return Object.entries(report.score_dimensions as Record<string, DimensionScore>)
      .map(([key, d]) => ({ key, label: d.label || key, score: d.score }))
      .sort((a, b) => a.key.localeCompare(b.key))
  }
  return [
    { key: 'architecture', label: '架构', score: report.score_arch ?? 0 },
    { key: 'quality', label: '质量', score: report.score_quality ?? 0 },
    { key: 'security', label: '安全', score: report.score_security ?? 0 },
    { key: 'maintainability', label: '可维护性', score: report.score_maint ?? 0 },
  ]
}

export default function PublicAuthorReportPage() {
  const { token } = useParams()
  const { data, isLoading, error } = useQuery({
    queryKey: ['public-author-report', token],
    queryFn: () => reviewsApi.publicAuthorGet(token!),
    enabled: !!token,
  })
  const { token: themeToken } = theme.useToken()
  const [severityFilter, setSeverityFilter] = useState('')

  const report = data?.report
  const findings = data?.findings || []
  const stats = useMemo(() => {
    if (!report?.stats) return null
    try { return JSON.parse(report.stats) } catch { return null }
  }, [report?.stats])

  if (isLoading) return <Center><Spin size="large" tip="加载报告中..." /></Center>
  if (error || !report) return <Center><Empty description="报告不存在或已被删除" /></Center>

  const display = report.author_name
    ? (report.author ? `${report.author_name} <${report.author}>` : report.author_name)
    : (report.author || '-')

  return (
    <Layout style={{ minHeight: '100vh', background: '#f0f2f5' }}>
      <Header style={{ background: themeToken.colorBgContainer, display: 'flex', alignItems: 'center', borderBottom: `1px solid ${themeToken.colorBorderSecondary}` }}>
        <Typography.Title level={4} style={{ margin: 0 }}>AI 代码审查报告 · 个人</Typography.Title>
      </Header>
      <Content style={{ maxWidth: 1100, width: '100%', margin: '0 auto', padding: 24 }}>
        <Card style={{ marginBottom: 16 }}>
          <Space size={48} wrap>
            <ScoreRing score={report.score_total} label="你的评分" size={140} />
            <div>
              {authorDimensionRows(report).map(d => (
                <div key={d.label} style={{ marginBottom: 12, minWidth: 220 }}>
                  <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                    <span>{d.label}</span><strong>{d.score}</strong>
                  </Space>
                  <div style={{ height: 8, background: '#f0f0f0', borderRadius: 4, overflow: 'hidden', marginTop: 4 }}>
                    <div style={{ height: '100%', width: `${d.score}%`, background: d.score >= 80 ? '#52c41a' : d.score >= 60 ? '#faad14' : '#f5222d' }} />
                  </div>
                </div>
              ))}
            </div>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="仓库">{report.repo_name}</Descriptions.Item>
              <Descriptions.Item label="提交者">{display}</Descriptions.Item>
              <Descriptions.Item label="分支">{report.target_ref || '-'}</Descriptions.Item>
              <Descriptions.Item label="Commit 区间">
                {(() => {
                  const actualBase = stats?.range_narrowed && stats?.range_base ? stats.range_base : report.base_sha
                  return actualBase
                    ? <Text code>{actualBase.slice(0, 8)}..{report.commit_sha?.slice(0, 8)}</Text>
                    : <Text code>{report.commit_sha?.slice(0, 12)}</Text>
                })()}
                {stats?.range_narrowed && (
                  <Typography.Text type="warning" style={{ marginLeft: 8, fontSize: 12 }}>
                    已收窄到最近 {stats.window_days || 5} 天
                  </Typography.Text>
                )}
              </Descriptions.Item>
              {stats?.range_start_at && stats?.range_end_at && (
                <Descriptions.Item label="提交时间">
                  {dayjs(stats.range_start_at).format('YYYY-MM-DD HH:mm')} ~ {dayjs(stats.range_end_at).format('YYYY-MM-DD HH:mm')}
                </Descriptions.Item>
              )}
              <Descriptions.Item label="变更">
                {report.files_changed} 文件 / +{report.additions} -{report.deletions}
                {stats?.files_limited ? <Typography.Text type="warning" style={{ marginLeft: 8, fontSize: 12 }}>（文件较多，仅审最近改动的 {stats.reviewed_files ?? '-'} 个）</Typography.Text> : null}
                {stats?.timed_out ? <Typography.Text type="danger" style={{ marginLeft: 8, fontSize: 12 }}>（达超时上限，按已分析内容出报告）</Typography.Text> : null}
              </Descriptions.Item>
              <Descriptions.Item label="你的问题">
                {report.findings_count}（critical {report.critical_count} / high {report.high_count} / medium {report.medium_count}）
              </Descriptions.Item>
              <Descriptions.Item label="触发时间">{dayjs(report.triggered_at).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
              <Descriptions.Item label="完成时间">
                {report.finished_at ? dayjs(report.finished_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        </Card>

        {report.summary && (
          <Card title="审查摘要" style={{ marginBottom: 16 }}>
            <Typography.Paragraph>{report.summary}</Typography.Paragraph>
          </Card>
        )}

        <Card title={`需要你关注的问题（${findings.length}）`}>
          <Space style={{ marginBottom: 16 }} wrap>
            {['', 'critical', 'high', 'medium', 'low', 'info'].map(s => (
              <Tag.CheckableTag key={s} checked={severityFilter === s} onChange={() => setSeverityFilter(s)}>
                {s ? <SeverityTag severity={s} /> : '全部'}
              </Tag.CheckableTag>
            ))}
          </Space>
          <Table
            rowKey="id"
            dataSource={findings.filter(f => !severityFilter || f.severity === severityFilter)}
            pagination={false}
            columns={[
              { title: '级别', dataIndex: 'severity', width: 90, render: (s: string) => <SeverityTag severity={s} /> },
              { title: '文件', dataIndex: 'file_path', width: 260, render: (f: string, r: Finding) => <Text code>{f}:{r.line_start}</Text> },
              { title: '问题', dataIndex: 'title', render: (t: string, r: Finding) => (
                <div><div>{t}</div>{r.suggestion && <Text type="secondary" style={{ fontSize: 12 }}>建议：{r.suggestion}</Text>}</div>
              )},
            ]}
            expandable={{
              expandedRowRender: (r: Finding) => (
                <div>
                  {r.message && <p style={{ marginBottom: 8 }}>{r.message}</p>}
                  {r.snippet && <pre style={{ background: '#f5f5f5', padding: 12, borderRadius: 4, fontSize: 12, overflowX: 'auto' }}>{r.snippet}</pre>}
                </div>
              ),
            }}
          />
          {findings.length === 0 && <Empty description="本次提交未发现归属到你的问题" />}
        </Card>
      </Content>
    </Layout>
  )
}

function Center({ children }: { children: React.ReactNode }) {
  return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>{children}</div>
}
