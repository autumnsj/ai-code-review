import { useMemo, useState } from 'react'
import {
  Modal, Steps, Form, Select, Input, Button, Table, Space, App, Typography, Alert, Tag, Checkbox,
} from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LinkOutlined } from '@ant-design/icons'
import { reposApi, ImportPreviewRepo, ImportResultRepo } from '../../api/repos'
import { credentialApi } from '../../api/credentials'
import { PROVIDER_OPTIONS, PROVIDER_BASE_PLACEHOLDER } from './index'

export default function ImportWizard({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [step, setStep] = useState(0)
  const [login, setLogin] = useState('')
  const [repos, setRepos] = useState<ImportPreviewRepo[]>([])
  const [selected, setSelected] = useState<number[]>([])
  const [hookSecret, setHookSecret] = useState('')
  const [results, setResults] = useState<{ results: ImportResultRepo[]; created: number; updated: number; failed: number } | null>(null)

  const { data: creds } = useQuery({ queryKey: ['credentials'], queryFn: credentialApi.list })

  const provider = Form.useWatch('provider', form)
  const apiBase = Form.useWatch('api_base_url', form)
  const credentialId = Form.useWatch('credential_id', form)

  const tokenCreds = useMemo(
    () => (creds ?? []).filter(c => c.type === 'https_token'),
    [creds],
  )

  const reset = () => {
    setStep(0); setLogin(''); setRepos([]); setSelected([]); setHookSecret(''); setResults(null)
    form.resetFields()
  }

  const preview = useMutation({
    mutationFn: reposApi.importPreview,
    onSuccess: (data) => {
      setLogin(data.login)
      setRepos(data.repos)
      // 默认选中所有未导入的仓库；已导入的也可手动勾选以重新同步（更新默认分支、重建 webhook）。
      setSelected(data.repos.map((r, i) => (!r.already_imported ? i : -1)).filter(i => i >= 0))
      setStep(1)
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '拉取仓库列表失败'),
  })

  const commit = useMutation({
    mutationFn: reposApi.importCommit,
    onSuccess: (data) => {
      setResults(data)
      setStep(2)
      qc.invalidateQueries({ queryKey: ['repos'] })
    },
    onError: (e: any) => message.error(e?.response?.data?.error || '导入失败'),
  })

  const doPreview = async () => {
    try {
      const v = await form.validateFields()
      preview.mutate({
        provider: v.provider,
        api_base_url: v.api_base_url || undefined,
        credential_id: v.credential_id,
      })
    } catch { /* 校验失败 */ }
  }

  const doCommit = () => {
    const items = selected.map(i => {
      const r = repos[i]
      return {
        name: r.name, clone_url: r.clone_url, web_url: r.web_url,
        default_branch: r.default_branch,
      }
    })
    if (items.length === 0) {
      message.warning('请至少选择一个仓库')
      return
    }
    commit.mutate({
      provider: form.getFieldValue('provider'),
      api_base_url: form.getFieldValue('api_base_url') || undefined,
      credential_id: form.getFieldValue('credential_id'),
      hook_secret: hookSecret || undefined,
      items,
    })
  }

  const close = () => { onClose(); setTimeout(reset, 200) }

  const footer = [
    <Button key="cancel" onClick={close}>关闭</Button>,
    ...(step === 0 ? [
      <Button key="next" type="primary" loading={preview.isPending} onClick={doPreview}>预览仓库列表</Button>,
    ] : []),
    ...(step === 1 ? [
      <Button key="back" onClick={() => setStep(0)}>上一步</Button>,
      <Button key="ok" type="primary" loading={commit.isPending} onClick={doCommit}>
        同步所选（{selected.length}）
      </Button>,
    ] : []),
  ]

  return (
    <Modal
      title="从平台批量导入仓库"
      open={open}
      onCancel={close}
      footer={footer}
      width={860}
      destroyOnClose
    >
      <Steps
        size="small"
        current={step}
        style={{ marginBottom: 16 }}
        items={[{ title: '选择凭据' }, { title: '勾选仓库' }, { title: '完成' }]}
      />

      {step === 0 && (
        <Form form={form} layout="vertical" initialValues={{ provider: 'github' }}>
          <Form.Item name="provider" label="平台" rules={[{ required: true }]}>
            <Select options={PROVIDER_OPTIONS} style={{ width: 240 }} />
          </Form.Item>
          <Form.Item
            name="api_base_url"
            label="API 地址"
            extra={provider === 'gitea' ? 'Gitea 为自建服务，必须填写实例地址' : '留空使用平台官方地址；GitHub Enterprise / 自建 GitLab 可填自定义地址'}
            rules={provider === 'gitea' ? [{ required: true, message: 'Gitea 需要填写 API 地址' }] : []}
          >
            <Input placeholder={PROVIDER_BASE_PLACEHOLDER[provider] || ''} />
          </Form.Item>
          <Form.Item
            name="credential_id"
            label="HTTPS Token 凭据"
            rules={[{ required: true, message: '请选择 Token 凭据' }]}
            extra={
              <span>
                需要一个有仓库读取权限的平台 Token。没有凭据？去
                <a href="/admin/credentials" target="_blank"> 凭据管理 </a>
                新建。
              </span>
            }
          >
            <Select
              placeholder="选择 HTTPS Token 凭据"
              options={tokenCreds.map(c => ({
                value: c.id,
                label: `${c.name}${c.provider ? `（${c.provider}）` : ''}`,
              }))}
              style={{ maxWidth: 420 }}
            />
          </Form.Item>
          {!tokenCreds.length && (
            <Alert
              type="warning"
              showIcon
              message="尚无 HTTPS Token 类型的凭据，请先在「凭据管理」中添加。"
              style={{ marginBottom: 8 }}
            />
          )}
        </Form>
      )}

      {step === 1 && (
        <>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
            已以 <b>@{login}</b> 身份拉取到 {repos.length} 个仓库。
            勾选后会<b>新建仓库记录并自动注册/更新 push webhook</b>，同时把平台的默认分支同步到本地。
            已导入的仓库也可再次勾选——会用平台最新值更新并<b>删旧重建 webhook</b>，不会跳过。
          </Typography.Paragraph>
          <div style={{ marginBottom: 8 }}>
            <Checkbox
              checked={selected.length === repos.length}
              indeterminate={selected.length > 0 && selected.length < repos.length}
              onChange={(e) => {
                if (e.target.checked) setSelected(repos.map((_, i) => i))
                else setSelected([])
              }}
            >
              全选（含已导入，用于重新同步）
            </Checkbox>
          </div>
          <Table
            size="small"
            rowKey={(_, i) => String(i)}
            dataSource={repos}
            pagination={{ pageSize: 8, showSizeChanger: false }}
            rowSelection={{
              selectedRowKeys: selected,
              onChange: (keys) => setSelected(keys.map(Number)),
            }}
            columns={[
              { title: '仓库', dataIndex: 'name', render: (t: string, r) => (
                <Space>
                  {r.web_url
                    ? <a href={r.web_url} target="_blank" rel="noreferrer">{t}<LinkOutlined /></a>
                    : t}
                  {r.private && <Tag color="orange">私有</Tag>}
                </Space>
              ) },
              { title: '默认分支', dataIndex: 'default_branch', width: 120 },
              { title: '状态', dataIndex: 'already_imported', width: 120, render: (v: boolean) =>
                v ? <Tag color="default">已导入（可同步）</Tag> : <Tag color="green">可导入</Tag> },
            ]}
          />
          <Input.Password
            style={{ marginTop: 12 }}
            placeholder="统一 Webhook Secret（可选，用于签名校验）"
            value={hookSecret}
            onChange={e => setHookSecret(e.target.value)}
            autoComplete="new-password"
          />
        </>
      )}

      {step === 2 && results && (
        <div>
          <Alert
            type={results.failed ? "warning" : "success"}
            showIcon
            message={
              results.failed
                ? `新建 ${results.created}、更新 ${results.updated}，其中 ${results.failed} 个 webhook 注册失败`
                : `完成：新建 ${results.created}、更新 ${results.updated}，webhook 均已注册`
            }
            style={{ marginBottom: 12 }}
          />
          <Table
            size="small"
            pagination={false}
            rowKey={(r) => `${r.id}-${r.name}`}
            dataSource={results.results}
            columns={[
              { title: '仓库', dataIndex: 'name' },
              { title: '动作', dataIndex: 'action', width: 90, render: (a: string) =>
                a === 'created' ? <Tag color="green">新建</Tag> : <Tag color="blue">更新</Tag> },
              { title: '默认分支', dataIndex: 'default_branch', width: 110,
                render: (b: string, r) => (
                  <Space size={4}>
                    {b || '-'}
                    {r.default_branch_changed && <Tag color="geekblue" style={{ marginInlineStart: 0 }}>已同步</Tag>}
                  </Space>
                ) },
              { title: 'Webhook', dataIndex: 'hook_registered', width: 220,
                render: (ok: boolean, r) => ok
                  ? <Tag color="green">已注册/更新</Tag>
                  : <Typography.Text type="danger">{r.hook_error || '未注册'}</Typography.Text> },
              { title: 'Hook URL', dataIndex: 'hook_url',
                render: (u: string) => <Typography.Text code copyable style={{ fontSize: 12 }}>{u}</Typography.Text> },
            ]}
          />
        </div>
      )}
    </Modal>
  )
}
