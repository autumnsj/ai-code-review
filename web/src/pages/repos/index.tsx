import { useMemo, useState } from 'react'
import {
  Button, Table, Space, Modal, Form, Input, Select, Typography, App, Tag, Tooltip,
} from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { PlusOutlined, CopyOutlined, ReloadOutlined, DeleteOutlined, CloudDownloadOutlined, ApiOutlined, SearchOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { reposApi, Repo } from '../../api/repos'
import { credentialApi } from '../../api/credentials'
import ImportWizard from './ImportWizard'

export const PROVIDER_OPTIONS = [
  { value: 'github', label: 'GitHub' },
  { value: 'gitlab', label: 'GitLab' },
  { value: 'gitee', label: 'Gitee（码云）' },
  { value: 'gitea', label: 'Gitea（自建）' },
]

export const PROVIDER_BASE_PLACEHOLDER: Record<string, string> = {
  github: 'https://api.github.com（留空用默认）',
  gitlab: 'https://gitlab.com（留空用默认）',
  gitee: 'https://gitee.com（留空用默认）',
  gitea: 'https://gitea.example.com（必填）',
}

export default function ReposPage() {
  const [open, setOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [keyword, setKeyword] = useState('')
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { message, modal } = App.useApp()
  const { data = [], isLoading } = useQuery({ queryKey: ['repos'], queryFn: reposApi.list })

  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return data
    return data.filter(r =>
      r.name.toLowerCase().includes(kw) ||
      r.provider.toLowerCase().includes(kw) ||
      r.clone_url.toLowerCase().includes(kw),
    )
  }, [data, keyword])

  const remove = useMutation({
    mutationFn: reposApi.remove,
    onSuccess: () => { message.success('已删除'); qc.invalidateQueries({ queryKey: ['repos'] }) },
  })

  const resetToken = useMutation({
    mutationFn: reposApi.resetToken,
    onSuccess: (d) => {
      message.success('已重置 hookToken')
      modal.info({
        title: '新的 hookUrl',
        content: <Input readOnly value={d.hook_url} />,
      })
      qc.invalidateQueries({ queryKey: ['repos'] })
    },
  })

  const registerWebhook = useMutation({
    mutationFn: reposApi.registerWebhook,
    onSuccess: (d) => {
      message.success(d.already_exists ? 'Webhook 已用最新配置更新' : 'Webhook 已注册（push 默认分支自动触发审查）')
      modal.info({
        title: d.already_exists ? '已更新的回调地址' : '已注册的回调地址',
        content: <Input readOnly value={d.hook_url} />,
      })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '注册失败'),
  })

  const registerAll = useMutation({
    mutationFn: reposApi.registerAllWebhooks,
    onSuccess: (d) => {
      const failed = d.items.filter(i => i.error)
      const changed = d.items.filter(i => i.default_branch_changed)
      const summary = `新增 ${d.created}，已存在 ${d.existed}，跳过 ${d.skipped}，失败 ${d.failed}`
      modal.info({
        title: '全部仓库 Webhook 注册结果',
        width: 640,
        content: (
          <div>
            <p style={{ marginTop: 0 }}>{summary}（共 {d.total} 个）</p>
            <p style={{ marginTop: 0 }}>
              <Tag color="green">默认分支已同步</Tag>
              已从平台读取各仓库真实默认分支并回写
              {changed.length > 0 ? `，其中 ${changed.length} 个本地记录被更新：` : '，无变更。'}
            </p>
            {changed.length > 0 && (
              <div style={{ maxHeight: 200, overflowY: 'auto', marginBottom: 8 }}>
                {changed.map(i => (
                  <div key={i.repo_id} style={{ marginBottom: 4 }}>
                    <Tag>{i.repo_name}</Tag>
                    <Typography.Text code style={{ fontSize: 12 }}>{i.default_branch}</Typography.Text>
                  </div>
                ))}
              </div>
            )}
            {failed.length > 0 && (
              <div style={{ maxHeight: 260, overflowY: 'auto' }}>
                {failed.map(i => (
                  <div key={i.repo_id} style={{ marginBottom: 8 }}>
                    <Tag color="red">{i.repo_name}</Tag>
                    <Typography.Text type="danger" style={{ fontSize: 12 }}>{i.error}</Typography.Text>
                  </div>
                ))}
              </div>
            )}
            {d.skipped > 0 && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                跳过的仓库未绑定 HTTPS Token 凭据，请在仓库里绑定后重试。
              </Typography.Text>
            )}
          </div>
        ),
      })
      qc.invalidateQueries({ queryKey: ['repos'] })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '批量注册失败'),
  })

  const copy = (text: string) => {
    navigator.clipboard.writeText(text)
    message.success('已复制')
  }

  // 一键审查：直接审查该仓库默认分支的最新提交（mode=repo）。
  const [pendingReviewId, setPendingReviewId] = useState<number | null>(null)
  const triggerReview = useMutation({
    mutationFn: (id: number) => reposApi.trigger(id, { mode: 'repo' }),
    onMutate: (id) => setPendingReviewId(id),
    onSuccess: (d) => {
      message.success('已发起审查，正在后台执行')
      navigate(`/admin/reviews/${d.review_id}`)
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '发起审查失败'),
    onSettled: () => setPendingReviewId(null),
  })

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', width: 240, ellipsis: true, render: (t: string, r: Repo) => <Link to={`/admin/repos/${r.id}`}>{t}</Link> },
    { title: '平台', dataIndex: 'provider', width: 90, render: (t: string) => <Tag>{t}</Tag> },
    { title: '分支', dataIndex: 'default_branch', width: 90 },
    {
      title: '凭据', dataIndex: 'credential_name', width: 160,
      render: (t: string) => t ? <Tag color="blue">{t}</Tag> : <Typography.Text type="secondary">内联 Token</Typography.Text>,
    },
    {
      title: 'hookUrl', dataIndex: 'hook_url', ellipsis: true,
      render: (u: string) => (
        <Space>
          <Typography.Text code style={{ fontSize: 12 }}>{u}</Typography.Text>
          <Tooltip title="复制"><Button size="small" icon={<CopyOutlined />} onClick={() => copy(u)} /></Tooltip>
        </Space>
      ),
    },
    {
      title: '操作', width: 340, render: (_: unknown, r: Repo) => (
        <Space>
          <Button
            size="small"
            type="primary"
            icon={<ThunderboltOutlined />}
            loading={pendingReviewId === r.id && triggerReview.isPending}
            onClick={() => triggerReview.mutate(r.id)}
          >
            审查
          </Button>
          <Button
            size="small"
            type="primary"
            ghost
            icon={<ApiOutlined />}
            loading={registerWebhook.isPending}
            onClick={() => registerWebhook.mutate(r.id)}
          >
            Webhook
          </Button>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => resetToken.mutate(r.id)}>重置</Button>
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => {
            modal.confirm({
              title: `删除仓库 ${r.name}？`,
              content: '相关审查记录与配置将一并删除。',
              onOk: () => remove.mutate(r.id),
            })
          }}>删除</Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Typography.Title level={3} style={{ margin: 0 }}>仓库管理</Typography.Title>
        <Space>
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜索仓库名 / 平台 / 地址"
            value={keyword}
            onChange={e => setKeyword(e.target.value)}
            style={{ width: 260 }}
          />
          <Button icon={<CloudDownloadOutlined />} onClick={() => setImportOpen(true)}>从平台导入</Button>
          <Button
            icon={<ApiOutlined />}
            loading={registerAll.isPending}
            onClick={() => registerAll.mutate()}
          >
            一键注册全部 Webhook
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>添加仓库</Button>
        </Space>
      </Space>
      <Table rowKey="id" loading={isLoading} dataSource={filtered} columns={columns} pagination={false} />
      <AddRepoModal open={open} onClose={() => setOpen(false)} />
      <ImportWizard open={importOpen} onClose={() => setImportOpen(false)} />
    </div>
  )
}

function AddRepoModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [form] = Form.useForm()
  const qc = useQueryClient()
  const { modal } = App.useApp()
  const { data: creds } = useQuery({ queryKey: ['credentials'], queryFn: credentialApi.list })
  const credentialId = Form.useWatch('credential_id', form)
  const selectedCred = creds?.find(c => c.id === credentialId)

  const create = useMutation({
    mutationFn: reposApi.create,
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['repos'] })
      modal.success({
        title: '仓库已添加',
        content: (
          <div>
            <p>请将下面的 hookUrl 配置到 Git 平台，此 token 仅展示一次：</p>
            <Input readOnly value={r.hook_url} />
          </div>
        ),
      })
      onClose()
      form.resetFields()
    },
  })
  return (
    <Modal title="添加仓库" open={open} onCancel={onClose} onOk={() => form.submit()} confirmLoading={create.isPending} destroyOnClose>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) => {
          // 0 表示「无（内联 Token）」；后端只认 >0 的 credential_id
          const payload = { ...v, credential_id: v.credential_id || undefined }
          create.mutate(payload)
        }}
        initialValues={{ provider: 'github', default_branch: 'main', credential_id: 0 }}
      >
        <Form.Item name="provider" label="平台" rules={[{ required: true }]}>
          <Select options={PROVIDER_OPTIONS} />
        </Form.Item>
        <Form.Item name="name" label="仓库名称" rules={[{ required: true }]}>
          <Input placeholder="owner/repo" />
        </Form.Item>
        <Form.Item name="clone_url" label="Clone URL" rules={[{ required: true }]}
          extra={selectedCred?.type === 'ssh' ? '已选 SSH 凭据，克隆地址需为 git@host:org/repo.git 形式' : undefined}>
          <Input placeholder={selectedCred?.type === 'ssh' ? 'git@github.com:owner/repo.git' : 'https://github.com/owner/repo.git'} />
        </Form.Item>
        <Form.Item name="web_url" label="Web URL">
          <Input placeholder="https://github.com/owner/repo" />
        </Form.Item>
        <Form.Item name="default_branch" label="默认分支">
          <Input placeholder="main" />
        </Form.Item>
        <Form.Item name="credential_id" label="凭据（clone 鉴权）">
          <Select
            allowClear={false}
            options={[
              { value: 0, label: '无（使用下方内联 Token）' },
              ...(creds ?? []).map(c => ({
                value: c.id,
                label: `${c.name}（${c.type === 'ssh' ? 'SSH 密钥' : 'HTTPS Token'}）`,
              })),
            ]}
          />
        </Form.Item>
        <Form.Item name="access_token" label="内联 Access Token（未选凭据时用于私有仓库）"
          tooltip="选择凭据后此项被忽略；留空则匿名 clone 公开仓库。">
          <Input.Password placeholder="可选" autoComplete="new-password" />
        </Form.Item>
        <Form.Item name="hook_secret" label="Webhook Secret（签名校验）">
          <Input.Password placeholder="可选" autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
