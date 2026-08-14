import { useState } from 'react'
import {
  Card, Steps, Radio, Form, Input, InputNumber, Button, Space, Typography, App, Alert, Divider,
} from 'antd'
import { setupApi } from '../../api/setup'

const { Title, Paragraph, Text } = Typography

type Driver = 'sqlite' | 'postgres'

interface PGForm {
  host: string
  port: number
  user: string
  password: string
  dbname: string
  sslmode: string
}

function buildPostgresDSN(f: PGForm): string {
  return `postgres://${encodeURIComponent(f.user)}:${encodeURIComponent(f.password)}@${f.host}:${f.port}/${f.dbname}?sslmode=${f.sslmode}`
}

export default function SetupPage() {
  const { message } = App.useApp()
  const [step, setStep] = useState(0)
  const [driver, setDriver] = useState<Driver>('sqlite')
  const [dsn, setDsn] = useState('')
  const [testing, setTesting] = useState(false)
  const [tested, setTested] = useState<null | boolean>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const onTest = async () => {
   	setTesting(true)
    try {
      let finalDSN = dsn
      if (driver === 'postgres') {
        if (!dsn) {
          const v = await form.validateFields()
          finalDSN = buildPostgresDSN(v)
        }
      }
      const res = await setupApi.test({ driver, dsn: finalDSN })
      setTested(res.ok)
      if (res.ok) message.success('连接成功')
      else message.error(`连接失败：${res.error}`)
    } catch (e: any) {
      setTested(false)
      message.error(e?.response?.data?.error || e?.message || '连接失败')
    } finally {
      setTesting(false)
    }
  }

  const onFinish = async (v: any) => {
    if (!tested) {
      message.warning('请先测试连接')
      return
    }
    setSubmitting(true)
    try {
      let finalDSN = dsn
      if (driver === 'postgres' && !dsn) finalDSN = buildPostgresDSN(v)
      await setupApi.complete({
        driver,
        dsn: finalDSN,
        admin_password: v.admin_password,
        base_url: v.base_url,
      })
      message.success('初始化完成，正在进入登录页…')
      setTimeout(() => (location.href = '/login'), 800)
    } catch (e: any) {
      message.error(e?.response?.data?.error || e?.message || '初始化失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', background: '#f0f2f5', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
      <Card style={{ width: 720 }}>
        <Title level={3} style={{ marginTop: 0 }}>AI Code Review 初始化</Title>
        <Paragraph type="secondary">首次使用，请选择数据库并创建管理员账号。数据库连接信息保存在数据目录下的 <Text code>aicr.json</Text>。</Paragraph>
        <Steps current={step} size="small" style={{ margin: '16px 0 24px' }}
          items={[{ title: '数据库' }, { title: '管理员' }]} />

        <Form form={form} layout="vertical" onFinish={onFinish}>
          {step === 0 && (
            <>
              <Form.Item label="数据库类型">
                <Radio.Group value={driver} onChange={e => { setDriver(e.target.value); setTested(null) }}>
                  <Radio.Button value="sqlite">SQLite（内置，零依赖）</Radio.Button>
                  <Radio.Button value="postgres">PostgreSQL</Radio.Button>
                </Radio.Group>
              </Form.Item>

              {driver === 'sqlite' && (
                <Alert type="info" showIcon message="使用内置 SQLite" description="数据文件位于数据目录下的 aicr.db（通过 AICR_DATA_DIR 或容器卷 /data 配置）。适合单机与中小团队。" />
              )}

              {driver === 'postgres' && (
                <>
                  <Paragraph type="secondary">填写 PostgreSQL 连接信息。若目标数据库不存在，会尝试自动创建。</Paragraph>
                  <Space style={{ width: '100%' }} size="middle" wrap>
                    <Form.Item name="host" label="主机" initialValue="localhost" rules={[{ required: true }]}>
                      <Input style={{ width: 200 }} />
                    </Form.Item>
                    <Form.Item name="port" label="端口" initialValue={5432}>
                      <InputNumber min={1} max={65535} style={{ width: 110 }} />
                    </Form.Item>
                    <Form.Item name="dbname" label="数据库名" initialValue="aicr" rules={[{ required: true }]}>
                      <Input style={{ width: 150 }} />
                    </Form.Item>
                  </Space>
                  <Space style={{ width: '100%' }} size="middle" wrap>
                    <Form.Item name="user" label="用户名" initialValue="postgres" rules={[{ required: true }]}>
                      <Input style={{ width: 180 }} />
                    </Form.Item>
                    <Form.Item name="password" label="密码">
                      <Input.Password style={{ width: 200 }} autoComplete="new-password" />
                    </Form.Item>
                    <Form.Item name="sslmode" label="SSL" initialValue="disable">
                      <Input style={{ width: 120 }} placeholder="disable/require" />
                    </Form.Item>
                  </Space>
                  <Divider plain><Text type="secondary">或直接粘贴 DSN</Text></Divider>
                  <Form.Item label="PostgreSQL DSN（可选，优先使用）">
                    <Input.TextArea rows={2} placeholder="postgres://user:pass@host:5432/aicr?sslmode=disable"
                      value={dsn} onChange={e => { setDsn(e.target.value); setTested(null) }} />
                  </Form.Item>
                </>
              )}

              <Space>
                <Button onClick={onTest} loading={testing}>测试连接</Button>
                <Button type="primary" disabled={!tested} onClick={() => setStep(1)}>下一步</Button>
              </Space>
            </>
          )}

          {step === 1 && (
            <>
              <Form.Item label="管理员密码" name="admin_password" rules={[{ required: true, min: 6, message: '密码至少 6 位' }]}>
                <Input.Password autoComplete="new-password" placeholder="至少 6 位" />
              </Form.Item>
              <Form.Item label="确认密码" name="confirm" dependencies={['admin_password']} rules={[
                { required: true },
                ({ getFieldValue }) => ({
                  validator(_, v) { return !v || v === getFieldValue('admin_password') ? Promise.resolve() : Promise.reject(new Error('两次密码不一致')) },
                }),
              ]}>
                <Input.Password autoComplete="new-password" />
              </Form.Item>
              <Form.Item label="对外基础地址" name="base_url" rules={[{ required: true, type: 'url', message: '请输入合法 URL' }]}
                extra="通知卡片中的报告链接将使用该地址，请填团队可访问的地址">
                <Input placeholder="http://localhost:8080" />
              </Form.Item>
              <Space>
                <Button onClick={() => setStep(0)}>上一步</Button>
                <Button type="primary" htmlType="submit" loading={submitting}>完成初始化</Button>
              </Space>
            </>
          )}
        </Form>
      </Card>
    </div>
  )
}
