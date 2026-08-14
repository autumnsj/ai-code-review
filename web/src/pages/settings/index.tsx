import { Button, Card, Form, Input, InputNumber, Radio, Select, Switch, Tabs, Typography, App, Space, Popconfirm } from 'antd'
import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { settingsApi, NotifierChannelInput, NotifierType, LLMProfileInput } from '../../api/settings'

export default function SettingsPage() {
  return (
    <div>
      <Typography.Title level={3}>系统设置</Typography.Title>
      <Tabs
        items={[
          { key: 'llm', label: 'AI 模型', children: <LLMPane /> },
          { key: 'notifications', label: '通知', children: <NotificationsPane /> },
          { key: 'server', label: '服务', children: <ServerPane /> },
          { key: 'security', label: '安全', children: <SecurityPane /> },
        ]}
      />
    </div>
  )
}

type LLMFormRow = LLMProfileInput & { api_key_set?: boolean }

function LLMPane() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm<{ profiles: LLMFormRow[]; default_id: string }>()
  const { data, isLoading } = useQuery({
    queryKey: ['settings-llm'],
    queryFn: settingsApi.getLLM,
  })
  useEffect(() => {
    if (data) {
      form.setFieldsValue({
        default_id: data.default_id,
        profiles: data.profiles.map((p) => ({
          id: p.id,
          name: p.name,
          base_url: p.base_url,
          model: p.model,
          api_key: '', // 留空不修改
          api_key_set: p.api_key_set,
          temperature: p.temperature || 0.2,
          max_tokens: p.max_tokens || 4096,
          timeout_sec: p.timeout_sec || 120,
          context_window: p.context_window || 64000,
          enabled: p.enabled,
        })),
      })
    }
  }, [data, form])

  const save = useMutation({
    mutationFn: (v: { profiles: LLMFormRow[]; default_id: string }) =>
      settingsApi.updateLLM({
        default_id: v.default_id,
        profiles: v.profiles.map(({ id, name, base_url, model, api_key, temperature, max_tokens, timeout_sec, context_window, enabled }) => ({
          id, name, base_url, model, api_key, temperature, max_tokens, timeout_sec, context_window, enabled,
        })),
      }),
    onSuccess: () => {
      message.success('已保存')
      qc.invalidateQueries({ queryKey: ['settings-llm'] })
    },
  })

  const newProfile = (): LLMFormRow => ({
    id: (crypto as any).randomUUID ? (crypto as any).randomUUID() : String(Date.now()),
    name: '', base_url: '', model: '', api_key: '',
    temperature: 0.2, max_tokens: 4096, timeout_sec: 120, context_window: 64000, enabled: true,
  })

  return (
    <Card loading={isLoading}>
      <Typography.Paragraph type="secondary">
        可配置多个 OpenAI 兼容模型，并选择一个作为<b>默认模型</b>（审查时使用）。API Key 脱敏保存，留空表示不修改。
      </Typography.Paragraph>
      <Form form={form} layout="vertical" onFinish={(v) => save.mutate(v)}>
        <Form.List name="profiles">
          {(fields, { add, remove }) => (
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              {fields.map(({ key, name }) => (
                <Card
                  key={key}
                  size="small"
                  title={
                    <Space>
                      <Form.Item name={[name, 'id']} noStyle hidden><Input /></Form.Item>
                      <Form.Item noStyle shouldUpdate>
                        {() => (
                          <Radio
                            checked={form.getFieldValue('default_id') === form.getFieldValue(['profiles', name, 'id'])}
                            onChange={() => form.setFieldValue('default_id', form.getFieldValue(['profiles', name, 'id']))}
                          >
                            默认
                          </Radio>
                        )}
                      </Form.Item>
                      <Form.Item name={[name, 'name']} noStyle>
                        <Input placeholder="模型名称（如 Qwen Heretic）" style={{ width: 240 }} variant="borderless" />
                      </Form.Item>
                    </Space>
                  }
                  extra={
                    <Space>
                      <Form.Item name={[name, 'enabled']} valuePropName="checked" noStyle>
                        <Switch checkedChildren="启用" unCheckedChildren="停用" />
                      </Form.Item>
                      <Popconfirm title="删除该模型？" onConfirm={() => remove(name)}>
                        <Button danger size="small">删除</Button>
                      </Popconfirm>
                    </Space>
                  }
                >
                  <Form.Item label="Base URL" name={[name, 'base_url']} rules={[{ required: true }]}>
                    <Input placeholder="https://api.openai.com/v1" />
                  </Form.Item>
                  <Form.Item label="模型" name={[name, 'model']} rules={[{ required: true }]}>
                    <Input placeholder="deepseek-chat / gpt-4o / qwen-plus ..." />
                  </Form.Item>
                  <Form.Item noStyle shouldUpdate={(p, c) =>
                    p.profiles?.[name]?.api_key_set !== c.profiles?.[name]?.api_key_set
                  }>
                    {() => (
                      <Form.Item
                        label="API Key" name={[name, 'api_key']}
                        extra={form.getFieldValue(['profiles', name, 'api_key_set'])
                          ? '已设置，留空不修改' : '尚未设置'}
                      >
                        <Input.Password autoComplete="new-password" placeholder="sk-..." />
                      </Form.Item>
                    )}
                  </Form.Item>
                  <Space style={{ width: '100%' }} size="middle" wrap>
                    <Form.Item label="Temperature" name={[name, 'temperature']}>
                      <InputNumber min={0} max={2} step={0.1} style={{ width: 130 }} />
                    </Form.Item>
                    <Form.Item label="最大 Tokens" name={[name, 'max_tokens']}>
                      <InputNumber min={256} step={256} style={{ width: 140 }} />
                    </Form.Item>
                    <Form.Item label="上下文窗口" name={[name, 'context_window']}>
                      <InputNumber min={1024} step={1024} style={{ width: 150 }} />
                    </Form.Item>
                    <Form.Item label="超时(秒)" name={[name, 'timeout_sec']}>
                      <InputNumber min={10} step={10} style={{ width: 110 }} />
                    </Form.Item>
                  </Space>
                </Card>
              ))}
              <Button onClick={() => add(newProfile())}>+ 添加模型</Button>
            </Space>
          )}
        </Form.List>
        <Button type="primary" htmlType="submit" loading={save.isPending} style={{ marginTop: 16 }}>保存</Button>
      </Form>
    </Card>
  )
}

const NOTIFIER_LABELS: Record<NotifierType, string> = {
  wecom: '企业微信',
  feishu: '飞书',
  dingtalk: '钉钉',
}

type NotifierFormRow = NotifierChannelInput & { secret_set?: boolean }

function NotificationsPane() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm<{ items: NotifierFormRow[] }>()
  const { data, isLoading } = useQuery({
    queryKey: ['settings-notifiers'],
    queryFn: settingsApi.getNotifiers,
  })
  useEffect(() => {
    if (data) {
      form.setFieldsValue({
        items: data.items.map((c) => ({
          type: c.type,
          webhook_url: c.webhook_url,
          secret: '', // 留空不修改
          secret_set: c.secret_set,
          enabled: c.enabled,
        })),
      })
    }
  }, [data, form])

  const save = useMutation({
    mutationFn: (rows: NotifierFormRow[]) =>
      settingsApi.updateNotifiers(
        rows.map(({ type, webhook_url, secret, enabled }) => ({ type, webhook_url, secret, enabled })),
      ),
    onSuccess: () => {
      message.success('已保存')
      qc.invalidateQueries({ queryKey: ['settings-notifiers'] })
    },
  })

  return (
    <Card loading={isLoading} style={{ maxWidth: 820 }}>
      <Typography.Paragraph type="secondary">
        审查完成后向启用的渠道推送 markdown 卡片。飞书/钉钉加签机器人需要填写「签名密钥」；企业微信群机器人通常无需 secret。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={(v) => save.mutate(v.items ?? [])}
      >
        <Form.List name="items">
          {(fields, { add, remove }) => (
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              {fields.map(({ key, name }) => (
                <Card key={key} size="small" title={`渠道 ${name + 1}`} extra={
                  <Popconfirm title="删除该渠道？" onConfirm={() => remove(name)}>
                    <Button danger size="small">删除</Button>
                  </Popconfirm>
                }>
                  <Form.Item
                    label="类型" name={[name, 'type']} rules={[{ required: true }]}
                    initialValue="wecom"
                  >
                    <Select style={{ width: 140 }} options={
                      (Object.keys(NOTIFIER_LABELS) as NotifierType[])
                        .map((t) => ({ value: t, label: NOTIFIER_LABELS[t] }))
                    } />
                  </Form.Item>
                  <Form.Item
                    label="Webhook 地址" name={[name, 'webhook_url']} rules={[{ required: true, type: 'url' }]}
                  >
                    <Input placeholder="https://..." />
                  </Form.Item>
                  <Form.Item noStyle shouldUpdate={(p, c) =>
                    p.items?.[name]?.secret_set !== c.items?.[name]?.secret_set
                  }>
                    {() => (
                      <Form.Item
                        label="签名密钥" name={[name, 'secret']}
                        extra={form.getFieldValue(['items', name, 'secret_set'])
                          ? '已设置，留空不修改' : '无加签可留空'}
                      >
                        <Input.Password autoComplete="new-password" placeholder="加签 secret（可选）" />
                      </Form.Item>
                    )}
                  </Form.Item>
                  <Form.Item label="启用" name={[name, 'enabled']} valuePropName="checked" initialValue={true}>
                    <Switch checkedChildren="启用" unCheckedChildren="停用" />
                  </Form.Item>
                </Card>
              ))}
              <Button onClick={() => add({ type: 'wecom', webhook_url: '', secret: '', enabled: true })}>
                + 添加渠道
              </Button>
            </Space>
          )}
        </Form.List>
        <Button type="primary" htmlType="submit" loading={save.isPending} style={{ marginTop: 16 }}>
          保存
        </Button>
      </Form>
    </Card>
  )
}

function ServerPane() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const { data, isLoading } = useQuery({
    queryKey: ['settings-server'],
    queryFn: settingsApi.getServer,
  })
  useEffect(() => {
    if (data) form.setFieldsValue({ base_url: data.base_url })
  }, [data, form])
  const save = useMutation({
    mutationFn: settingsApi.updateServer,
    onSuccess: () => {
      message.success('已保存')
      qc.invalidateQueries({ queryKey: ['settings-server'] })
    },
  })
  return (
    <Card loading={isLoading} style={{ maxWidth: 640 }}>
      <Form form={form} layout="vertical" onFinish={(v) => save.mutate(v)}>
        <Form.Item label="对外基础地址" name="base_url" rules={[{ required: true }]} extra="通知卡片中的报告链接使用该地址，请填团队可访问的地址">
          <Input placeholder="https://review.example.com" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={save.isPending}>保存</Button>
      </Form>
    </Card>
  )
}

function SecurityPane() {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const change = useMutation({
    mutationFn: settingsApi.changePassword,
    onSuccess: () => {
      message.success('密码已修改，请重新登录')
      setTimeout(() => {
        localStorage.removeItem('token')
        location.href = '/login'
      }, 1000)
    },
  })
  return (
    <Card style={{ maxWidth: 640 }}>
      <Form form={form} layout="vertical" onFinish={(v) => change.mutate(v)}>
        <Form.Item label="原密码" name="old_password" rules={[{ required: true }]}>
          <Input.Password />
        </Form.Item>
        <Form.Item label="新密码" name="new_password" rules={[{ required: true, min: 6 }]}>
          <Input.Password />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={change.isPending}>修改密码</Button>
      </Form>
    </Card>
  )
}
