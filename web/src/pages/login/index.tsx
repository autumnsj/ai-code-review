import { Button, Card, Form, Input, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { http } from '../../api/client'

export default function LoginPage() {
  const nav = useNavigate()
  const onFinish = async (v: { password: string }) => {
    const { data } = await http.post('/api/admin/login', v)
    localStorage.setItem('token', data.token)
    nav('/admin')
  }
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 360 }}>
        <Typography.Title level={3} style={{ textAlign: 'center' }}>AI Code Review</Typography.Title>
        <Form onFinish={onFinish}>
          <Form.Item name="password" rules={[{ required: true, message: '请输入管理员密码' }]}>
            <Input.Password placeholder="管理员密码" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block size="large">登录</Button>
        </Form>
      </Card>
    </div>
  )
}
