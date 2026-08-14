import { Card, Col, Row, Statistic, Table, Tag, Typography, List } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { opsApi } from '../../api/ops'
import { statsApi } from '../../api/stats'
import { scoreColor, StatusTag } from '../../components/SeverityTag'

export default function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: opsApi.dashboard,
    refetchInterval: 10000,
  })
  const { data: topAuthors } = useQuery({
    queryKey: ['dashboard-top-authors'],
    queryFn: () => statsApi.listAuthors({ days: 30, sort: 'avg_score', page_size: 5 }),
  })

  return (
    <div>
      <Typography.Title level={3}>概览</Typography.Title>
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={4}><Card><Statistic title="仓库数" value={data?.repo_count ?? 0} loading={isLoading} /></Card></Col>
        <Col span={4}><Card><Statistic title="审查总数" value={data?.review_count ?? 0} loading={isLoading} /></Card></Col>
        <Col span={4}><Card><Statistic title="成功" valueStyle={{ color: '#3f8600' }} value={data?.succeeded ?? 0} loading={isLoading} /></Card></Col>
        <Col span={4}><Card><Statistic title="失败" valueStyle={{ color: '#cf1322' }} value={data?.failed ?? 0} loading={isLoading} /></Card></Col>
        <Col span={4}><Card><Statistic title="进行中" valueStyle={{ color: '#1677ff' }} value={data?.pending ?? 0} loading={isLoading} /></Card></Col>
        <Col span={4}><Card><Statistic title="平均评分" valueStyle={{ color: scoreColor(data?.avg_score ?? 0) }} value={data?.avg_score ?? 0} precision={1} loading={isLoading} /></Card></Col>
      </Row>

      <Row gutter={16}>
        <Col span={16}>
          <Card title="最近审查">
            <Table
              rowKey="id"
              loading={isLoading}
              dataSource={data?.recent_reviews ?? []}
              pagination={false}
              columns={[
                { title: 'ID', dataIndex: 'id', width: 70, render: (v: number) => <Link to={`/admin/reviews/${v}`}>#{v}</Link> },
                { title: '仓库', dataIndex: 'repo_name' },
                { title: 'Commit', dataIndex: 'commit_sha', width: 100, render: (v: string) => <code>{v}</code> },
                { title: '分支', dataIndex: 'target_ref', width: 120, render: (v: string) => v ? <Tag>{v}</Tag> : '-' },
                {
                  title: '评分', dataIndex: 'score_total', width: 90, align: 'center' as const,
                  render: (v: number, r) => r.status === 'succeeded'
                    ? <span style={{ color: scoreColor(v), fontWeight: 600 }}>{v}</span> : '-',
                },
                { title: '状态', dataIndex: 'status', width: 110, render: (v: string) => <StatusTag status={v} /> },
                { title: '完成时间', dataIndex: 'finished_at', width: 160, render: (v: string) => v ?? '-' },
              ]}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card title="近 30 天作者均分 Top 5" extra={<Link to="/admin/stats/authors">更多</Link>}>
            <List
              size="small"
              dataSource={topAuthors?.items ?? []}
              locale={{ emptyText: '暂无数据' }}
              renderItem={(a, i) => (
                <List.Item>
                  <List.Item.Meta
                    avatar={<Tag color={i === 0 ? 'gold' : 'default'}>{i + 1}</Tag>}
                    title={<Link to="/admin/stats/authors">{a.author}</Link>}
                    description={`${a.review_count} 次审查 · +${a.additions}/-${a.deletions}`}
                  />
                  <span style={{ color: scoreColor(a.avg_total), fontWeight: 600, fontSize: 16 }}>{a.avg_total.toFixed(1)}</span>
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
