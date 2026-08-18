import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, Descriptions, Table, Typography, Tag, Space, Button, App, Modal, Form, Input, Select, Switch, Radio, Popconfirm } from 'antd'
import { CopyOutlined, EditOutlined, LinkOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useState } from 'react'
import dayjs from 'dayjs'
import { reposApi } from '../../api/repos'
import { reviewsApi, Review } from '../../api/reviews'
import { credentialApi } from '../../api/credentials'
import { StatusTag } from '../../components/SeverityTag'

export default function RepoDetailPage() {
  const { id } = useParams()
  const repoId = Number(id)
  const qc = useQueryClient()
  const { message } = App.useApp()
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
    mutationFn: (v: any) => reposApi.trigger(repoId, v),
    onSuccess: () => {
      message.success('已触发审查')
      setOpen(false)
      qc.invalidateQueries({ queryKey: ['reviews', repoId] })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '触发失败'),
  })

  // 重新审查：对已有记录强制重跑（force=true），复用同一条 review 行。
  const recheck = useMutation({
    mutationFn: (r: Review) => reposApi.trigger(repoId, {
      mode: 'commit',
      commit_sha: r.commit_sha,
      base_sha: r.base_sha || undefined,
      target_ref: r.target_ref || undefined,
      force: true,
    }),
    onSuccess: () => {
      message.success('已重新提交审查')
      qc.invalidateQueries({ queryKey: ['reviews', repoId] })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '重新审查失败'),
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
            { title: '操作', width: 110, render: (_: unknown, r: Review) =>
              ['pending', 'running'].includes(r.status) ? null : (
                <Popconfirm
                  title="重新审查该提交？"
                  description="将覆盖当前这条审查记录的结果。"
                  okText="重新审查"
                  cancelText="取消"
                  onConfirm={() => recheck.mutate(r)}
                >
                  <Button type="link" size="small" icon={<ReloadOutlined />} loading={recheck.isPending}>重新审查</Button>
                </Popconfirm>
              ) },
          ]}
        />
      </Card>

      <TriggerModal
        open={open}
        onClose={() => setOpen(false)}
        onSubmit={(v) => trigger.mutate(v)}
        loading={trigger.isPending}
        defaultBranch={repo?.default_branch}
        hasCredential={(repo?.credential_id ?? 0) > 0}
      />

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

function TriggerModal({
  open, onClose, onSubmit, loading, defaultBranch, hasCredential,
}: {
  open: boolean
  onClose: () => void
  onSubmit: (v: Record<string, unknown>) => void
  loading: boolean
  defaultBranch?: string
  hasCredential: boolean
}) {
  const [form] = Form.useForm()
  const mode = Form.useWatch('mode', form) || 'commit'
  return (
    <Modal
      title="手动触发审查"
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={loading}
      destroyOnClose
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ mode: 'commit' }}
        onFinish={(v) => {
          const payload: Record<string, unknown> = { mode: v.mode }
          if (v.mode === 'commit') {
            payload.commit_sha = v.commit_sha
            payload.base_sha = v.base_sha
            payload.target_ref = v.target_ref
          } else if (v.mode === 'branch') {
            payload.ref = v.ref
            payload.base_sha = v.base_sha
          }
          onSubmit(payload)
        }}
      >
        <Form.Item label="目标" name="mode">
          <Radio.Group>
            <Radio.Button value="commit">指定 Commit</Radio.Button>
            <Radio.Button value="branch">指定分支</Radio.Button>
            <Radio.Button value="repo">整个仓库</Radio.Button>
          </Radio.Group>
        </Form.Item>
        {mode === 'commit' && (
          <>
            <Form.Item label="Commit SHA" name="commit_sha" rules={[{ required: true, message: '请输入完整 commit SHA' }]}>
              <Input placeholder="要审查的 commit SHA" />
            </Form.Item>
            <Form.Item label="目标分支/ref" name="target_ref" extra={defaultBranch ? `留空使用默认分支 ${defaultBranch}` : undefined}>
              <Input placeholder="master / main / refs/heads/..." />
            </Form.Item>
            <Form.Item label="Base SHA（可选，用于 diff 对比）" name="base_sha">
              <Input placeholder="对比基线 commit" />
            </Form.Item>
          </>
        )}
        {mode === 'branch' && (
          <>
            {!hasCredential && (
              <Typography.Paragraph type="warning" style={{ marginBottom: 12 }}>
                分支模式需要仓库绑定 HTTPS Token 凭据以解析分支 HEAD。
              </Typography.Paragraph>
            )}
            <Form.Item label="分支名" name="ref" rules={[{ required: true, message: '请输入分支名' }]}>
              <Input placeholder="feature / develop / main" />
            </Form.Item>
            <Form.Item label="Base SHA（可选）" name="base_sha">
              <Input placeholder="对比基线 commit" />
            </Form.Item>
          </>
        )}
        {mode === 'repo' && (
          <Typography.Paragraph type="secondary">
            将解析默认分支{defaultBranch ? `（${defaultBranch}）` : ''}当前 HEAD 进行全量审查。
            {!hasCredential && ' 注意：需要仓库绑定 HTTPS Token 凭据。'}
          </Typography.Paragraph>
        )}
      </Form>
    </Modal>
  )
}
