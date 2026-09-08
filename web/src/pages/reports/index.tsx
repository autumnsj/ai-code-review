import { Button, Modal, Select, Space, Table, Tag, Typography } from 'antd'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { reportsApi, type Report } from '../../api/reports'

const TEAM = '__team__' // 成员筛选值：团队简报/异常提醒（author 为空）

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
  const [authorFilter, setAuthorFilter] = useState<string | undefined>(undefined)
  const [viewId, setViewId] = useState<number | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['reports', page, authorFilter],
    queryFn: () =>
      reportsApi.list({
        page,
        page_size: 20,
        author: authorFilter === TEAM ? '' : authorFilter,
      }),
  })

  // 查看时才拉正文（列表接口不含 content）。
  const { data: detail, isLoading: detailLoading, isError: detailError } = useQuery({
    queryKey: ['report', viewId],
    queryFn: () => reportsApi.get(viewId as number),
    enabled: viewId !== null,
  })

  const items = data?.items ?? []
  // 成员筛选选项：从当前列表聚合（含「团队」桶）。
  const authorOptions = [
    ...new Set(items.map((r) => (r.author ? r.author : TEAM))),
  ].map((a) => ({ value: a, label: a === TEAM ? '团队（简报/异常）' : a }))

  return (
    <div>
      <Typography.Title level={3}>报告记录</Typography.Title>
      <Typography.Paragraph type="secondary">
        报告以<b>人</b>为单位（用于考核）：每位成员每期一份独立报告，推送到群时也是每人一条，内容只含本人完成的功能与名下问题；
        成员为「团队」的是无审查简报或失败/无主问题的异常提醒。定时发送与「立即发送」均记录，预览不记录。
      </Typography.Paragraph>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          showSearch
          placeholder="按成员筛选"
          style={{ width: 200 }}
          value={authorFilter}
          onChange={(v) => { setAuthorFilter(v); setPage(1) }}
          options={authorOptions}
        />
      </Space>
      <Table<Report>
        rowKey="id"
        loading={isLoading}
        dataSource={items}
        pagination={{ current: page, pageSize: 20, total: data?.total ?? 0, onChange: setPage, showSizeChanger: false }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          {
            title: '成员', dataIndex: 'author', width: 130,
            render: (v: string) => v
              ? <Tag color="blue">{v}</Tag>
              : <Tag>团队</Tag>,
          },
          {
            title: '类型', dataIndex: 'kind', width: 90,
            render: (v: string) => <Tag color={v === 'weekly' ? 'purple' : 'geekblue'}>{v === 'weekly' ? '周报' : '日报'}</Tag>,
          },
          { title: '统计周期', width: 230, render: (_, r) => periodText(r) },
          {
            title: '触发方式', dataIndex: 'trigger_type', width: 100,
            render: (v: string) => <Tag color={v === 'manual' ? 'gold' : 'default'}>{v === 'manual' ? '手动' : '定时'}</Tag>,
          },
          { title: '生成时间', dataIndex: 'created_at', width: 170, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm') },
          {
            title: '操作', width: 90,
            render: (_, r) => (
              <Button type="link" size="small" onClick={() => setViewId(r.id)}>查看</Button>
            ),
          },
        ]}
      />
      <Modal
        title={detail ? detail.title : '报告详情'}
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
