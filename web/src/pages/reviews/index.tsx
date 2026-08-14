import { Table, Typography, Tag } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import dayjs from 'dayjs'
import { reviewsApi, Review } from '../../api/reviews'
import { StatusTag } from '../../components/SeverityTag'

const scoreColor = (s: number) => (s >= 80 ? '#52c41a' : s >= 60 ? '#faad14' : '#f5222d')

export default function ReviewsPage() {
  const [params] = useSearchParams()
  const repoId = Number(params.get('repo_id')) || undefined
  const { data, isLoading } = useQuery({
    queryKey: ['reviews', repoId],
    queryFn: () => reviewsApi.list({ repo_id: repoId, page_size: 50 }),
    refetchInterval: 5000,
  })

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 70, render: (v: number, r: Review) => <Link to={`/admin/reviews/${r.id}`}>#{v}</Link> },
    { title: '仓库', dataIndex: 'repo_name', width: 200 },
    {
      title: '类型', dataIndex: 'event_type', width: 100,
      render: (t: string, r: Review) => (
        <span>
          <Tag>{t === 'pull_request' ? 'PR' : 'Push'}</Tag>
          {r.pr_title || r.commit_sha.slice(0, 8)}
        </span>
      ),
    },
    { title: '作者', dataIndex: 'author', width: 120 },
    {
      title: '评分', dataIndex: 'score_total', width: 90,
      render: (v: number, r: Review) =>
        r.status === 'succeeded'
          ? <strong style={{ color: scoreColor(v) }}>{v}</strong>
          : '—',
    },
    { title: '状态', dataIndex: 'status', width: 100, render: (s: string) => <StatusTag status={s} /> },
    {
      title: '时间', dataIndex: 'triggered_at', width: 180,
      render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm:ss'),
    },
  ]

  return (
    <div>
      <Typography.Title level={3}>审查记录</Typography.Title>
      <Table
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items || []}
        columns={columns}
        pagination={{ pageSize: 20, total: data?.total }}
      />
    </div>
  )
}
