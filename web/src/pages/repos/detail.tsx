import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, Descriptions, Table, Typography, Tag, Space, Button, App, Modal, Form, Input, Select, Switch } from 'antd'
import { CopyOutlined, EditOutlined, LinkOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useState } from 'react'
import dayjs from 'dayjs'
import { reposApi } from '../../api/repos'
import { reviewsApi, Review } from '../../api/reviews'
import { opsApi } from '../../api/ops'
import { credentialApi } from '../../api/credentials'
import { StatusTag } from '../../components/SeverityTag'

export default function RepoDetailPage() {
  const { id } = useParams()
  const repoId = Number(id)
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [open, setOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
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
          extra={
            <Space>
              <Button icon={<EditOutlined />} onClick={() => setEditOpen(true)}>编辑</Button>
              <Button type="primary" icon={<ThunderboltOutlined />} onClick={() => setOpen(true)}>手动触发审查</Button>
            </Space>
          }
        >
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="平台"><Tag>{repo.provider}</Tag></Descriptions.Item>
            <Descriptions.Item label="默认分支">{repo.default_branch}</Descriptions.Item>
            <Descriptions.Item label="clone 凭据">
              {repo.credential_name ? <Tag color="blue">{repo.credential_name}</Tag> : <Typography.Text type="secondary">内联 Token</Typography.Text>}
            </Descriptions.Item>
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

      <EditRepoModal
        open={editOpen}
        onClose={() => setEditOpen(false)}
        repoId={repoId}
      />
    </div>
  )
}

function EditRepoModal({ open, onClose, repoId }: { open: boolean; onClose: () => void; repoId: number }) {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const { data: repo } = useQuery({ queryKey: ['repo', repoId], queryFn: () => reposApi.get(repoId) })
  const { data: creds } = useQuery({ queryKey: ['credentials'], queryFn: credentialApi.list })
  const changeToken = Form.useWatch('change_token', form)
  const changeHook = Form.useWatch('change_hook', form)

  const update = useMutation({
    mutationFn: (v: any) => reposApi.update(repoId, v),
    onSuccess: () => {
      message.success('已保存')
      qc.invalidateQueries({ queryKey: ['repo', repoId] })
      qc.invalidateQueries({ queryKey: ['repos'] })
      onClose()
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '保存失败'),
  })

  return (
    <Modal
      title="编辑仓库"
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={update.isPending}
      destroyOnClose
      forceRender
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          name: repo?.name,
          default_branch: repo?.default_branch,
          credential_id: repo?.credential_id ?? 0,
          change_token: false,
          change_hook: false,
        }}
        onFinish={(v) => {
          const payload: Record<string, unknown> = {
            name: v.name,
            default_branch: v.default_branch,
            credential_id: v.credential_id || null,
          }
          if (v.change_token) payload.access_token = v.access_token ?? ''
          if (v.change_hook) payload.hook_secret = v.hook_secret ?? ''
          update.mutate(payload)
        }}
      >
        <Form.Item label="仓库名称" name="name" rules={[{ required: true }]}>
          <Input placeholder="owner/repo" />
        </Form.Item>
        <Form.Item label="默认分支" name="default_branch">
          <Input placeholder="main" />
        </Form.Item>
        <Form.Item label="凭据（clone 鉴权）" name="credential_id" extra="选「无」会解绑凭据，转而使用内联 Token。">
          <Select
            options={[
              { value: 0, label: '无（使用内联 Token）' },
              ...(creds ?? []).map(c => ({
                value: c.id,
                label: `${c.name}（${c.type === 'ssh' ? 'SSH 密钥' : 'HTTPS Token'}）`,
              })),
            ]}
          />
        </Form.Item>
        <Form.Item label="修改内联 Access Token" name="change_token" valuePropName="checked">
          <Switch checkedChildren="修改" unCheckedChildren="不改" />
        </Form.Item>
        {changeToken && (
          <Form.Item name="access_token" extra="留空保存将清除现有内联 Token。">
            <Input.Password placeholder="新的 Access Token" autoComplete="new-password" />
          </Form.Item>
        )}
        <Form.Item label="修改 Webhook Secret" name="change_hook" valuePropName="checked">
          <Switch checkedChildren="修改" unCheckedChildren="不改" />
        </Form.Item>
        {changeHook && (
          <Form.Item name="hook_secret" extra="留空保存将清除现有签名校验。">
            <Input.Password placeholder="新的 Webhook Secret" autoComplete="new-password" />
          </Form.Item>
        )}
      </Form>
    </Modal>
  )
}
