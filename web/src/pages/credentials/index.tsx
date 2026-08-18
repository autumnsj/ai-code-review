import { useState } from 'react'
import {
  App, Button, Form, Input, Modal, Popconfirm, Radio, Select, Space, Table, Tag, Typography,
} from 'antd'
import { CopyOutlined, DeleteOutlined, KeyOutlined, PlusOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { credentialApi, Credential, CredentialType } from '../../api/credentials'

const { Paragraph, Text } = Typography

export default function CredentialsPage() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { data, isLoading } = useQuery({ queryKey: ['credentials'], queryFn: credentialApi.list })
  const [modalOpen, setModalOpen] = useState(false)
  const [oneTimeKey, setOneTimeKey] = useState<{ name: string; key: string } | null>(null)

  const remove = useMutation({
    mutationFn: credentialApi.remove,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['credentials'] })
      qc.invalidateQueries({ queryKey: ['repos'] })
    },
  })

  const copy = (text: string, label = '已复制') => {
    navigator.clipboard.writeText(text).then(() => message.success(label))
  }

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Typography.Title level={3} style={{ margin: 0 }}>凭据</Typography.Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>添加凭据</Button>
      </Space>

      <Paragraph type="secondary">
        管理可跨仓库复用的 clone 凭据。SSH 凭据需使用 <Text code>git@host:org/repo.git</Text> 形式的克隆地址；
        将公钥添加到 Git 平台的 Deploy Keys。HTTPS Token 凭据对 <Text code>https://</Text> 克隆地址生效。
      </Paragraph>

      <Table
        rowKey="id"
        loading={isLoading}
        dataSource={data ?? []}
        pagination={false}
        columns={[
          { title: '名称', dataIndex: 'name', width: 200 },
          {
            title: '类型', dataIndex: 'type', width: 120,
            render: (t: CredentialType, r: Credential) => (
              <Space direction="vertical" size={0}>
                {t === 'ssh' ? <Tag color="blue">SSH 密钥</Tag> : <Tag>HTTPS Token</Tag>}
                {t === 'https_token' && r.provider && (
                  <Text type="secondary" style={{ fontSize: 12 }}>{r.provider}{r.api_base_url ? ` · ${r.api_base_url}` : ''}</Text>
                )}
              </Space>
            ),
          },
          {
            title: '指纹 / 掩码', key: 'ident',
            render: (_: any, r: Credential) => r.type === 'ssh'
              ? <Text code copyable={{ text: r.fingerprint }}>{r.fingerprint}</Text>
              : <Text code>{r.secret_masked || (r.secret_set ? '已设置' : '未设置')}</Text>,
          },
          {
            title: '公钥', dataIndex: 'public_key', ellipsis: true,
            render: (v: string) => v ? <Text code style={{ fontSize: 12 }}>{v}</Text> : '-',
          },
          { title: '创建时间', dataIndex: 'created_at', width: 140 },
          {
            title: '操作', width: 160, key: 'actions',
            render: (_: any, r: Credential) => (
              <Space>
                {r.type === 'ssh' && r.public_key && (
                  <Button size="small" icon={<CopyOutlined />} onClick={() => copy(r.public_key!, '公钥已复制')}>公钥</Button>
                )}
                <Popconfirm title="删除该凭据？引用它的仓库将解绑。" onConfirm={() => remove.mutate(r.id)}>
                  <Button size="small" danger icon={<DeleteOutlined />}>删除</Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <CreateCredentialModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onCreated={(name, priv) => {
          setModalOpen(false)
          qc.invalidateQueries({ queryKey: ['credentials'] })
          if (priv) setOneTimeKey({ name, key: priv })
        }}
      />

      <Modal
        title="私钥（仅显示一次）"
        open={!!oneTimeKey}
        onCancel={() => setOneTimeKey(null)}
        onOk={() => {
          if (oneTimeKey) copy(oneTimeKey.key, '私钥已复制')
        }}
        okText="复制私钥"
        cancelText="我已保存"
      >
        <Paragraph type="warning">
          这是凭据「{oneTimeKey?.name}」的私钥唯一一次显示，请立即复制并妥善保管。服务端不再返回明文。
        </Paragraph>
        <Input.TextArea value={oneTimeKey?.key ?? ''} readOnly autoSize={{ minRows: 6, maxRows: 12 }} />
      </Modal>
    </div>
  )
}

function CreateCredentialModal({
  open, onClose, onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated: (name: string, oneTimePrivateKey?: string) => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const type = Form.useWatch('type', form) ?? 'ssh'
  const sshMode = Form.useWatch('ssh_mode', form) ?? 'generate'

  const create = useMutation({
    mutationFn: credentialApi.create,
    onSuccess: (cred) => {
      message.success('凭据已创建')
      form.resetFields()
      onCreated(cred.name, cred.private_key)
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '创建失败'),
  })

  return (
    <Modal title="添加凭据" open={open} onCancel={onClose} confirmLoading={create.isPending}
      onOk={() => form.submit()} destroyOnClose>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ type: 'ssh', ssh_mode: 'generate', provider: 'github' }}
        onFinish={(v) => {
          const secret = v.type === 'ssh' ? (v.ssh_mode === 'paste' ? (v.private_key ?? '') : '') : (v.token ?? '')
          create.mutate({
            name: v.name, type: v.type, secret,
            provider: v.type === 'https_token' ? v.provider : undefined,
            api_base_url: v.type === 'https_token' ? (v.api_base_url || undefined) : undefined,
          })
        }}
      >
        <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
          <Input placeholder="如 github-deploy-key" />
        </Form.Item>
        <Form.Item label="类型" name="type">
          <Radio.Group>
            <Radio value="ssh"><KeyOutlined /> SSH 密钥</Radio>
            <Radio value="https_token">HTTPS Token</Radio>
          </Radio.Group>
        </Form.Item>

        {type === 'ssh' && (
          <>
            <Form.Item name="ssh_mode" noStyle>
              <Radio.Group style={{ marginBottom: 12 }}>
                <Radio value="generate">自动生成 ed25519 密钥对</Radio>
                <Radio value="paste">粘贴已有私钥</Radio>
              </Radio.Group>
            </Form.Item>
            {sshMode === 'paste' && (
              <Form.Item label="私钥 (PEM)" name="private_key" rules={[{ required: true, message: '请粘贴私钥' }]}>
                <Input.TextArea rows={6} placeholder="-----BEGIN OPENSSH PRIVATE KEY----- ..." />
              </Form.Item>
            )}
            {sshMode === 'generate' && (
              <Paragraph type="secondary" style={{ fontSize: 13 }}>
                将自动生成 ed25519 密钥对。创建后公钥显示在列表中，私钥仅展示一次供下载/复制。
              </Paragraph>
            )}
          </>
        )}

        {type === 'https_token' && (
          <>
            <Form.Item label="Token" name="token" rules={[{ required: true, message: '请输入 Token' }]}>
              <Input.Password placeholder="ghp_... / glpat-..." autoComplete="new-password" />
            </Form.Item>
            <Form.Item label="所属平台" name="provider" extra="用于「从平台导入仓库」与解析分支；同一平台可建多个凭据。">
              <Select options={[
                { value: 'github', label: 'GitHub' },
                { value: 'gitlab', label: 'GitLab' },
                { value: 'gitee', label: 'Gitee（码云）' },
                { value: 'gitea', label: 'Gitea（自建）' },
              ]} />
            </Form.Item>
            <Form.Item
              label="API 地址"
              name="api_base_url"
              extra="自建实例（如 Gitea / GH Enterprise / 自建 GitLab）才需要填写，官方平台留空。"
            >
              <Input placeholder="https://gitea.example.com" />
            </Form.Item>
          </>
        )}
      </Form>
    </Modal>
  )
}
