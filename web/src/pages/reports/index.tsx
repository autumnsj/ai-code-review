import { Button, Modal, Table, Tag, Typography } from 'antd'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { reportsApi, type Report } from '../../api/reports'

// periodText 格式化统计周期：日报取窗口起始日（全天），周报取 起～止（end 为排他上界，减一天）。
function periodText(r: Report): string {
  const start = dayjs(r.period_start)
  if (r.kind === 'weekly') {
    const end = dayjs(r.period_end).subtract(1, 'day')
    return `${start.format('YYYY-MM-DD')} ～ ${end.format('YYYY-MM-DD')}`
  }
  return start.format('YYYY-MM-DD')
}

export default function ReportsPage() {
  const [page, setPage] = useState(1)
  const [viewId, setViewId] = useState<number | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['reports', page],
    queryFn: () => reportsApi.list({ page, page_size: 20 }),
  })

  // 查看时才拉正文（列表接口不含 content）。
  const { data: detail, isLoading: detailLoading, isError: detailError } = useQuery({
    queryKey: ['report', viewId],
    queryFn: () => reportsApi.get(viewId as number),
    enabled: viewId !== null,
  })

  return (
    <div>
      <Typography.Title level={3}>报告记录</Typography.Title>
      <Typography.Paragraph type="secondary">
        每次发送的日报/周报都会留存（定时发送与「立即发送」均记录；预览不记录），内容与推送到群的一致。
      </Typography.Paragraph>
      <Table<Report>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{ current: page, pageSize: 20, total: data?.total ?? 0, onChange: setPage, showSizeChanger: false }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          {
            title: '类型', dataIndex: 'kind', width: 90,
            render: (v: string) => <Tag color={v === 'weekly' ? 'purple' : 'blue'}>{v === 'weekly' ? '周报' : '日报'}</Tag>,
          },
          { title: '统计周期', width: 240, render: (_, r) => periodText(r) },
          {
            title: '触发方式', dataIndex: 'trigger_type', width: 100,
            render: (v: string) => <Tag color={v === 'manual' ? 'gold' : 'default'}>{v === 'manual' ? '手动' : '定时'}</Tag>,
          },
          { title: '生成时间', dataIndex: 'created_at', width: 180, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
          {
            title: '操作', width: 90,
            render: (_, r) => (
              <Button type="link" size="small" onClick={() => setViewId(r.id)}>查看</Button>
            ),
          },
        ]}
      />
      <Modal
        title={detail ? `${detail.title}（${periodText(detail)}）` : '报告详情'}
        open={viewId !== null}
        onCancel={() => setViewId(null)}
        footer={null}
        width={720}
        loading={detailLoading}
      >
        {detailError ? (
          <Typography.Text type="danger">加载报告内容失败，请稍后重试。</Typography.Text>
        ) : (
          <div style={{ whiteSpace: 'pre-wrap', maxHeight: '60vh', overflowY: 'auto', lineHeight: 1.7 }}>
            {detail?.content}
          </div>
        )}
      </Modal>
    </div>
  )
}
