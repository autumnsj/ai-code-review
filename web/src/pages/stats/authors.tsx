import { useState } from 'react'
import {
  Card, Col, Row, Select, Space, Statistic, Table, Tag, Typography, Drawer, Descriptions, Progress,
  Empty,
} from 'antd'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { statsApi, AuthorSummary } from '../../api/stats'
import { reposApi, Repo } from '../../api/repos'
import { scoreColor } from '../../components/SeverityTag'

const { Title } = Typography

const DAYS_OPTIONS = [
  { value: 7, label: '近 7 天' },
  { value: 30, label: '近 30 天' },
  { value: 90, label: '近 90 天' },
  { value: 0, label: '全部' },
]

const SORT_OPTIONS = [
  { value: 'avg_score', label: '平均评分' },
  { value: 'review_count', label: '审查次数' },
  { value: 'additions', label: '新增行数' },
  { value: 'deletions', label: '删除行数' },
  { value: 'findings', label: '问题数量' },
]

function DimBar({ value }: { value: number }) {
  return <Progress percent={value} size="small" showInfo={false} strokeColor={scoreColor(value)} style={{ width: 90, margin: 0 }} />
}

export default function AuthorsStatsPage() {
  const [days, setDays] = useState(30)
  const [repoId, setRepoId] = useState<number | undefined>()
  const [sort, setSort] = useState('avg_score')
  const [active, setActive] = useState<AuthorSummary | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['stats-authors', days, repoId, sort],
    queryFn: () => statsApi.listAuthors({ days, repo_id: repoId, sort, page_size: 100 }),
  })
  const { data: repos } = useQuery({ queryKey: ['repos'], queryFn: reposApi.list })

  const { data: detail, isLoading: detailLoading } = useQuery({
    queryKey: ['stats-author', active?.author, days, repoId],
    queryFn: () => statsApi.getAuthor(active!.author, { days, repo_id: repoId }),
    enabled: !!active,
  })

  const items = data?.items ?? []
  const totals = items.reduce(
    (acc, a) => {
      acc.reviews += a.review_count
      acc.add += a.additions
      acc.del += a.deletions
      acc.findings += a.findings_total
      return acc
    },
    { reviews: 0, add: 0, del: 0, findings: 0 },
  )

  return (
    <div>
      <Title level={3}>作者排行</Title>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}><Card><Statistic title="作者数" value={items.length} loading={isLoading} /></Card></Col>
        <Col span={6}><Card><Statistic title="审查总次数" value={totals.reviews} loading={isLoading} /></Card></Col>
        <Col span={6}><Card><Statistic title="代码变动（+/-）" value={`+${totals.add} / -${totals.del}`} loading={isLoading} /></Card></Col>
        <Col span={6}><Card><Statistic title="问题总数" value={totals.findings} valueStyle={{ color: totals.findings > 0 ? '#cf1322' : undefined }} loading={isLoading} /></Card></Col>
      </Row>

      <Card
        title="作者维度统计"
        extra={
          <Space wrap>
            <Select style={{ width: 130 }} value={days} options={DAYS_OPTIONS} onChange={setDays} />
            <Select
              style={{ width: 200 }} allowClear placeholder="全部仓库"
              value={repoId} onChange={setRepoId}
              options={(repos as Repo[] | undefined)?.map(r => ({ value: r.id, label: r.name })) ?? []}
            />
            <Select style={{ width: 140 }} value={sort} options={SORT_OPTIONS} onChange={setSort} />
          </Space>
        }
      >
        <Table
          rowKey="author"
          loading={isLoading}
          dataSource={items}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          onRow={(r) => ({ onClick: () => setActive(r), style: { cursor: 'pointer' } })}
          columns={[
            {
              title: '作者', dataIndex: 'author', width: 180,
              render: (v: string) => <span style={{ fontWeight: 600 }}>{v}</span>,
            },
            { title: '审查次数', dataIndex: 'review_count', width: 100, sorter: (a, b) => a.review_count - b.review_count },
            {
              title: '平均分', dataIndex: 'avg_total', width: 110, sorter: (a, b) => a.avg_total - b.avg_total,
              render: (v: number) => <span style={{ color: scoreColor(v), fontWeight: 600 }}>{v.toFixed(1)}</span>,
            },
            {
              title: '架构', dataIndex: 'avg_arch', width: 120,
              render: (v: number) => <DimBar value={v} />,
            },
            { title: '质量', dataIndex: 'avg_quality', width: 120, render: (v: number) => <DimBar value={v} /> },
            { title: '安全', dataIndex: 'avg_security', width: 120, render: (v: number) => <DimBar value={v} /> },
            { title: '可维护', dataIndex: 'avg_maint', width: 120, render: (v: number) => <DimBar value={v} /> },
            {
              title: '代码量', key: 'code', width: 160,
              render: (_: any, r) => <span><span style={{ color: '#3f8600' }}>+{r.additions}</span> / <span style={{ color: '#cf1322' }}>-{r.deletions}</span></span>,
            },
            {
              title: '问题', key: 'findings', width: 200,
              render: (_: any, r) => (
                <Space size={4} wrap>
                  {r.critical > 0 && <Tag color="red">C {r.critical}</Tag>}
                  {r.high > 0 && <Tag color="orange">H {r.high}</Tag>}
                  {r.medium > 0 && <Tag color="gold">M {r.medium}</Tag>}
                  {r.low > 0 && <Tag>L {r.low}</Tag>}
                  {r.findings_total === 0 && <span style={{ color: '#999' }}>无</span>}
                </Space>
              ),
            },
            { title: '最近审查', dataIndex: 'last_reviewed', width: 140 },
          ]}
        />
      </Card>

      <Drawer
        title={active ? `作者：${active.author}` : ''}
        open={!!active} width={560} onClose={() => setActive(null)}
        loading={detailLoading}
      >
        {detail && (
          <>
            <Descriptions column={2} bordered size="small" title="汇总">
              <Descriptions.Item label="审查次数">{detail.summary.review_count}</Descriptions.Item>
              <Descriptions.Item label="平均分">
                <span style={{ color: scoreColor(detail.summary.avg_total), fontWeight: 600 }}>{detail.summary.avg_total.toFixed(1)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="新增">{detail.summary.additions}</Descriptions.Item>
              <Descriptions.Item label="删除">{detail.summary.deletions}</Descriptions.Item>
              <Descriptions.Item label="变更文件">{detail.summary.files_changed}</Descriptions.Item>
              <Descriptions.Item label="Token 用量">{detail.summary.tokens_used}</Descriptions.Item>
              <Descriptions.Item label="问题总数" span={2}>{detail.summary.findings_total}（严重 {detail.summary.critical} / 高 {detail.summary.high} / 中 {detail.summary.medium}）</Descriptions.Item>
            </Descriptions>

            <Card title="维度均分" size="small" style={{ marginTop: 16 }}>
              {(['avg_arch', 'avg_quality', 'avg_security', 'avg_maint'] as const).map((k) => {
                const labels: Record<string, string> = { avg_arch: '架构', avg_quality: '质量', avg_security: '安全', avg_maint: '可维护性' }
                return (
                  <div key={k} style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
                    <span style={{ width: 72 }}>{labels[k]}</span>
                    <Progress percent={detail.summary[k]} style={{ flex: 1, margin: 0 }} strokeColor={scoreColor(detail.summary[k])} />
                    <span style={{ width: 48, textAlign: 'right' }}>{detail.summary[k].toFixed(1)}</span>
                  </div>
                )
              })}
            </Card>

            <Card title="最近审查" size="small" style={{ marginTop: 16 }}>
              {detail.recent.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无" /> : (
                <Table
                  rowKey="id" size="small" pagination={false}
                  dataSource={detail.recent}
                  columns={[
                    { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Link to={`/admin/reviews/${v}`}>#{v}</Link> },
                    { title: '仓库', dataIndex: 'repo_name' },
                    { title: '评分', dataIndex: 'score_total', width: 70, render: (v: number) => <span style={{ color: scoreColor(v) }}>{v}</span> },
                    { title: '变动', key: 'c', width: 110, render: (_: any, r: any) => `+${r.additions}/-${r.deletions}` },
                    { title: '时间', dataIndex: 'finished_at', width: 120 },
                  ]}
                />
              )}
            </Card>
          </>
        )}
      </Drawer>
    </div>
  )
}
