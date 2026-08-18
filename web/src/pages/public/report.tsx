import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Layout, Card, Descriptions, Table, Tag, Space, Typography, Empty, Spin, theme,
} from 'antd'
import dayjs from 'dayjs'
import { reviewsApi, Finding, dimensionRows } from '../../api/reviews'
import ScoreRing from '../../components/ScoreRing'
import SeverityTag from '../../components/SeverityTag'

const { Text } = Typography
const { Header, Content } = Layout

export default function PublicReportPage() {
  const { token } = useParams()
  const { data, isLoading, error } = useQuery({
    queryKey: ['public-report', token],
    queryFn: () => reviewsApi.publicGet(token!),
    enabled: !!token,
    refetchInterval: (q) => {
      const s = q.state.data?.review?.status
      return s === 'running' || s === 'pending' ? 5000 : false
    },
  })
  const { token: themeToken } = theme.useToken()
  const [severityFilter, setSeverityFilter] = useState('')

  const review = data?.review
  const findings = data?.findings || []

  const stats = useMemo(() => {
    if (!review?.stats) return null
    try { return JSON.parse(review.stats) } catch { return null }
  }, [review?.stats])

  if (isLoading) return <Center><Spin size="large" tip="加载报告中..." /></Center>
  if (error || !review) return <Center><Empty description="报告不存在或已被删除" /></Center>

  return (
    <Layout style={{ minHeight: '100vh', background: '#f0f2f5' }}>
      <Header style={{ background: themeToken.colorBgContainer, display: 'flex', alignItems: 'center', borderBottom: `1px solid ${themeToken.colorBorderSecondary}` }}>
        <Typography.Title level={4} style={{ margin: 0 }}>AI 代码审查报告</Typography.Title>
      </Header>
      <Content style={{ maxWidth: 1100, width: '100%', margin: '0 auto', padding: 24 }}>
        <Card style={{ marginBottom: 16 }}>
          <Space size={48} wrap>
            <ScoreRing score={review.score_total} label="综合评分" size={140} />
            <div>
              {dimensionRows(review).map(d => (
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
              <Descriptions.Item label="仓库">{review.repo_name}</Descriptions.Item>
              <Descriptions.Item label="分支">{review.target_ref || '-'}</Descriptions.Item>
              <Descriptions.Item label="Commit 区间">
                {review.base_sha
                  ? <Text code>{review.base_sha.slice(0, 8)}..{review.commit_sha.slice(0, 8)}</Text>
                  : <Text code>{review.commit_sha.slice(0, 12)}</Text>}
                <Typography.Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                  (head {review.commit_sha.slice(0, 8)}{review.base_sha ? `，base ${review.base_sha.slice(0, 8)}` : '，首次提交'})
                </Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label="作者">{review.author || '-'}</Descriptions.Item>
              {review.pr_title && <Descriptions.Item label="PR">{review.pr_title}</Descriptions.Item>}
              {stats?.range_start_at && stats?.range_end_at && (
                <Descriptions.Item label="提交时间">
                  {dayjs(stats.range_start_at).format('YYYY-MM-DD HH:mm')} ~ {dayjs(stats.range_end_at).format('YYYY-MM-DD HH:mm')}
                  {stats.range_narrowed ? <Typography.Text type="warning" style={{ marginLeft: 8, fontSize: 12 }}>（已收窄到最近 {stats.window_days || 5} 天）</Typography.Text> : null}
                </Descriptions.Item>
              )}
              {stats && (
                <Descriptions.Item label="变更">
                  {stats.files_changed} 文件 / +{stats.additions} -{stats.deletions}
                  {stats.files_limited ? <Typography.Text type="warning" style={{ marginLeft: 8, fontSize: 12 }}>（文件较多，仅审最近改动的 {stats.reviewed_files ?? '-'} 个）</Typography.Text> : null}
                  {stats.timed_out ? <Typography.Text type="danger" style={{ marginLeft: 8, fontSize: 12 }}>（达超时上限，按已分析内容出报告）</Typography.Text> : null}
                </Descriptions.Item>
              )}
              <Descriptions.Item label="触发时间">{dayjs(review.triggered_at).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
              {review.started_at && (
                <Descriptions.Item label="开始时间">{dayjs(review.started_at).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
              )}
              <Descriptions.Item label="完成时间">
                {review.finished_at ? dayjs(review.finished_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        </Card>

        {review.summary && (
          <Card title="审查摘要" style={{ marginBottom: 16 }}>
            <Typography.Paragraph>{review.summary}</Typography.Paragraph>
          </Card>
        )}

        {review.status === 'succeeded' && (
          <Card title={`问题清单（${findings.length}）`}>
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
            {findings.length === 0 && <Empty description="未发现问题" />}
          </Card>
        )}

        {(review.status === 'running' || review.status === 'pending') && (
          <Card><Spin tip="审查进行中，页面将自动刷新..." /></Card>
        )}
        {review.status === 'failed' && (
          <Card><Typography.Text type="danger">审查失败：{review.error}</Typography.Text></Card>
        )}
      </Content>
    </Layout>
  )
}

function Center({ children }: { children: React.ReactNode }) {
  return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>{children}</div>
}
