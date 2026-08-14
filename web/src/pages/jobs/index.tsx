import { Button, Select, Space, Table, Tag, Typography, App } from 'antd'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { opsApi, Job } from '../../api/ops'

const STATUS_COLORS: Record<string, string> = {
  pending: 'default', running: 'processing', succeeded: 'success', failed: 'error', dead: 'red',
}

export default function JobsPage() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [status, setStatus] = useState<string | undefined>(undefined)
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['jobs', status, page],
    queryFn: () => opsApi.listJobs({ status, page, page_size: 20 }),
    refetchInterval: 5000,
  })

  const retry = useMutation({
    mutationFn: opsApi.retryJob,
    onSuccess: () => {
      message.success('已重新入队')
      qc.invalidateQueries({ queryKey: ['jobs'] })
    },
    onError: () => message.error('重试失败'),
  })

  return (
    <div>
      <Typography.Title level={3}>任务队列</Typography.Title>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear placeholder="按状态筛选" style={{ width: 160 }} value={status}
          onChange={(v) => { setStatus(v); setPage(1) }}
          options={[
            { value: 'pending', label: '排队中' },
            { value: 'running', label: '执行中' },
            { value: 'succeeded', label: '成功' },
            { value: 'failed', label: '失败（待重试）' },
            { value: 'dead', label: '已放弃' },
          ]}
        />
      </Space>
      <Table<Job>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{ current: page, pageSize: 20, total: data?.total ?? 0, onChange: setPage }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          { title: '类型', dataIndex: 'kind', width: 100 },
          {
            title: '状态', dataIndex: 'status', width: 130,
            render: (v: string) => <Tag color={STATUS_COLORS[v] || 'default'}>{v}</Tag>,
          },
          { title: '尝试', dataIndex: 'attempts', width: 80, render: (v: number, r) => `${v}/${r.max_attempts}` },
          {
            title: '错误', dataIndex: 'last_error', ellipsis: true,
            render: (v: string) => v ? <code style={{ color: '#cf1322' }}>{v}</code> : '-',
          },
          { title: '创建时间', dataIndex: 'created_at', width: 200, render: (v: string) => new Date(v).toLocaleString() },
          {
            title: '操作', width: 100,
            render: (_: any, r: Job) => (
              (r.status === 'failed' || r.status === 'dead') &&
              <Button size="small" onClick={() => retry.mutate(r.id)} loading={retry.isPending}>重试</Button>
            ),
          },
        ]}
      />
    </div>
  )
}
