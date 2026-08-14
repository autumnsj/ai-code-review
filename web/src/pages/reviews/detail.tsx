import { useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Card, Descriptions, Tabs, Table, Tag, Space, Typography, Empty, Spin, Button, Input, Alert,
} from 'antd'
import { LinkOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { reviewsApi, Finding } from '../../api/reviews'
import ScoreRing from '../../components/ScoreRing'
import SeverityTag from '../../components/SeverityTag'
import { StatusTag } from '../../components/SeverityTag'

const { Text } = Typography

export default function ReviewDetailPage() {
  const { id } = useParams()
  const reviewId = Number(id)
  const [severityFilter, setSeverityFilter] = useState<string>('')
  const [fileFilter, setFileFilter] = useState('')

  const { data: review } = useQuery({
    queryKey: ['review', reviewId],
    queryFn: () => reviewsApi.get(reviewId),
    refetchInterval: (q) => (q.state.data?.status === 'running' || q.state.data?.status === 'pending' ? 3000 : false),
  })
  const { data: findings = [], isLoading } = useQuery({
    queryKey: ['findings', reviewId],
    queryFn: () => reviewsApi.findings(reviewId),
    enabled: review?.status === 'succeeded',
  })

  const stats = useMemo(() => {
    if (!review?.stats) return null
    try { return JSON.parse(review.stats) } catch { return null }
  }, [review?.stats])

  const filtered = findings.filter(f =>
    (!severityFilter || f.severity === severityFilter) &&
    (!fileFilter || f.file_path.toLowerCase().includes(fileFilter.toLowerCase()))
  )

  if (!review) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Space>
          <Link to="/admin/reviews">← 返回</Link>
          <Typography.Title level={4} style={{ margin: 0 }}>审查 #{review.id}</Typography.Title>
          <StatusTag status={review.status} />
        </Space>
        {review.status === 'succeeded' && (
          <Button icon={<LinkOutlined />} href={`/reports/${review.public_token}`} target="_blank">公开报告</Button>
        )}
      </Space>

      {review.error && <Alert type="error" message="审查失败" description={review.error} showIcon style={{ marginBottom: 16 }} />}

      {(review.status === 'running' || review.status === 'pending') && (
        <Card><Spin tip="审查进行中..." /></Card>
      )}

      {review.status === 'succeeded' && (
        <>
          <Card style={{ marginBottom: 16 }}>
            <Space size={48} wrap>
              <ScoreRing score={review.score_total} label="综合评分" size={140} />
              <div>
                <ScoreBars review={review} />
              </div>
              <Descriptions column={1} size="small">
                <Descriptions.Item label="仓库">{review.repo_name}</Descriptions.Item>
                <Descriptions.Item label="Commit"><Text code>{review.commit_sha.slice(0, 12)}</Text></Descriptions.Item>
                <Descriptions.Item label="作者">{review.author}</Descriptions.Item>
                {stats && <Descriptions.Item label="变更">{stats.files_changed} 文件 / +{stats.additions} -{stats.deletions}</Descriptions.Item>}
                <Descriptions.Item label="耗时">
                  {review.started_at && review.finished_at
                    ? `${dayjs(review.finished_at).diff(dayjs(review.started_at), 'second')}s` : '-'}
                </Descriptions.Item>
                {review.tokens_used > 0 && <Descriptions.Item label="Token 用量">{review.tokens_used}</Descriptions.Item>}
              </Descriptions>
            </Space>
          </Card>

          {review.summary && (
            <Card title="审查摘要" style={{ marginBottom: 16 }}>
              <Typography.Paragraph>{review.summary}</Typography.Paragraph>
            </Card>
          )}

          <Card title={`问题清单（${findings.length}）`}>
            <Space style={{ marginBottom: 16 }}>
              <span>严重度：</span>
              {['', 'critical', 'high', 'medium', 'low', 'info'].map(s => (
                <Tag.CheckableTag key={s} checked={severityFilter === s} onChange={() => setSeverityFilter(s)}>
                  {s ? <SeverityTag severity={s} /> : '全部'}
                </Tag.CheckableTag>
              ))}
              <Input.Search placeholder="按文件名过滤" allowClear style={{ width: 240 }} onSearch={setFileFilter} />
            </Space>
            <Table
              rowKey="id"
              loading={isLoading}
              dataSource={filtered}
              pagination={false}
              size="middle"
              columns={[
                { title: '级别', dataIndex: 'severity', width: 90, render: (s: string) => <SeverityTag severity={s} /> },
                { title: '来源', dataIndex: 'source', width: 80, render: (s: string) => <Tag>{s}</Tag> },
                { title: '文件', dataIndex: 'file_path', width: 240, render: (f: string, r: Finding) => <Text code>{f}:{r.line_start}</Text> },
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
            {filtered.length === 0 && !isLoading && <Empty />}
          </Card>
        </>
      )}
    </div>
  )
}

function ScoreBars({ review }: { review: any }) {
  const dims = [
    { label: '架构', score: review.score_arch },
    { label: '质量', score: review.score_quality },
    { label: '安全', score: review.score_security },
    { label: '可维护性', score: review.score_maint },
  ]
  return (
    <div style={{ minWidth: 240 }}>
      {dims.map(d => (
        <div key={d.label} style={{ marginBottom: 12 }}>
          <Space style={{ justifyContent: 'space-between', width: '100%' }}>
            <span>{d.label}</span><strong>{d.score}</strong>
          </Space>
          <div style={{ height: 8, background: '#f0f0f0', borderRadius: 4, overflow: 'hidden', marginTop: 4 }}>
            <div style={{ height: '100%', width: `${d.score}%`, background: d.score >= 80 ? '#52c41a' : d.score >= 60 ? '#faad14' : '#f5222d' }} />
          </div>
        </div>
      ))}
    </div>
  )
}
