import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, Descriptions, Table, Typography, Tag, Space, Button, App, Modal, Form, Input } from 'antd'
import { CopyOutlined, LinkOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useState } from 'react'
import dayjs from 'dayjs'
import { reposApi } from '../../api/repos'
import { reviewsApi, Review } from '../../api/reviews'
import { opsApi } from '../../api/ops'
import { StatusTag } from '../../components/SeverityTag'

export default function RepoDetailPage() {
  const { id } = useParams()
  const repoId = Number(id)
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [open, setOpen] = useState(false)
  const { data: repo } = useQuery({ queryKey: ['repo', repoId], queryFn: () => reposApi.get(repoId) })
  const { data: reviews } = useQuery({
    queryKey: ['reviews', repoId],
    queryFn: () => reviewsApi.list({ repo_id: repoId, page_size: 20 }),
    refetchInterval: 5000,
  })

  const copy = (t: string) => { navigator.clipboard.writeText(t); message.success('已复制') }

  const trigger = useMutation({
    mutationFn: (v: { commit_sha: string; base_sha?: string; target_ref?: string }) => opsApi.trigger(repoId, v),
    onSuccess: () => {
      message.success('已触发审查')
      setOpen(false)
      form.resetFields()
      qc.invalidateQueries({ queryKey: ['reviews', repoId] })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '触发失败'),
  })

  return (
    <div>
      <Typography.Title level={3}>{repo?.name || `仓库 #${id}`}</Typography.Title>
      {repo && (
        <Card
          title="基本信息"
          style={{ marginBottom: 16 }}
          extra={<Button type="primary" icon={<ThunderboltOutlined />} onClick={() => setOpen(true)}>手动触发审查</Button>}
        >
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="平台"><Tag>{repo.provider}</Tag></Descriptions.Item>
            <Descriptions.Item label="默认分支">{repo.default_branch}</Descriptions.Item>
            <Descriptions.Item label="Clone URL" span={2}>{repo.clone_url}</Descriptions.Item>
            <Descriptions.Item label="hookUrl" span={2}>
              <Space>
                <Typography.Text code style={{ fontSize: 12 }}>{repo.hook_url}</Typography.Text>
                <Button size="small" icon={<CopyOutlined />} onClick={() => copy(repo.hook_url)}>复制</Button>
              </Space>
            </Descriptions.Item>
            <Descriptions.Item label="签名校验">{repo.has_secret ? <Tag color="green">已配置</Tag> : <Tag>未配置</Tag>}</Descriptions.Item>
            <Descriptions.Item label="状态"><Tag color={repo.status === 'active' ? 'green' : 'red'}>{repo.status}</Tag></Descriptions.Item>
          </Descriptions>
        </Card>
      )}
      <Card title="审查记录">
        <Table
          rowKey="id"
          size="small"
          dataSource={reviews?.items || []}
          pagination={false}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 70, render: (v: number) => <a href={`/admin/reviews/${v}`}>#{v}</a> },
            { title: '类型', dataIndex: 'event_type', width: 90, render: (t: string, r: Review) => r.pr_title || (t === 'manual' ? '手动' : t === 'pull_request' ? 'PR' : 'Push') },
            { title: 'Commit', dataIndex: 'commit_sha', width: 120, render: (s: string) => <Typography.Text code>{s.slice(0, 10)}</Typography.Text> },
            { title: '评分', dataIndex: 'score_total', width: 80, render: (v: number, r: Review) => r.status === 'succeeded' ? v : '—' },
            { title: '状态', dataIndex: 'status', width: 100, render: (s: string) => <StatusTag status={s} /> },
            { title: '时间', dataIndex: 'triggered_at', render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
            { title: '报告', width: 90, render: (_: unknown, r: Review) =>
              r.status === 'succeeded' ? <Button type="link" size="small" icon={<LinkOutlined />} href={`/reports/${r.public_token}`} target="_blank">查看</Button> : null },
          ]}
        />
      </Card>

      <Modal
        title="手动触发审查"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={trigger.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical" onFinish={(v) => trigger.mutate(v)}>
          <Form.Item label="Commit SHA" name="commit_sha" rules={[{ required: true, message: '请输入完整 commit SHA' }]}>
            <Input placeholder="要审查的 commit SHA（必填）" />
          </Form.Item>
          <Form.Item label="目标分支/ref" name="target_ref" extra={repo?.default_branch ? `留空使用默认分支 ${repo.default_branch}` : undefined}>
            <Input placeholder="master / main / refs/heads/..." />
          </Form.Item>
          <Form.Item label="Base SHA（可选，用于 diff 对比）" name="base_sha">
            <Input placeholder="对比基线 commit" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
