import { useState } from 'react'
import {
  Button, Table, Space, Modal, Form, Input, Select, Typography, App, Tag, Tooltip,
} from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { PlusOutlined, CopyOutlined, ReloadOutlined, DeleteOutlined } from '@ant-design/icons'
import { reposApi, Repo } from '../../api/repos'
import { credentialApi } from '../../api/credentials'

export default function ReposPage() {
  const [open, setOpen] = useState(false)
  const qc = useQueryClient()
  const { message, modal } = App.useApp()
  const { data = [], isLoading } = useQuery({ queryKey: ['repos'], queryFn: reposApi.list })

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

  const copy = (text: string) => {
    navigator.clipboard.writeText(text)
    message.success('已复制')
  }

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name', render: (t: string, r: Repo) => <Link to={`/admin/repos/${r.id}`}>{t}</Link> },
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
      title: '操作', width: 180, render: (_: unknown, r: Repo) => (
        <Space>
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
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>添加仓库</Button>
      </Space>
      <Table rowKey="id" loading={isLoading} dataSource={data} columns={columns} pagination={false} />
      <AddRepoModal open={open} onClose={() => setOpen(false)} />
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
          <Select options={[
            { value: 'github', label: 'GitHub' },
            { value: 'gitlab', label: 'GitLab' },
            { value: 'gitee', label: 'Gitee' },
            { value: 'coding', label: 'Coding' },
          ]} />
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
