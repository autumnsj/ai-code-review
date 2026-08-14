import { Button, Card, Form, Input, Typography } from 'antd'
import { GithubOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { http } from '../../api/client'
import { AUTHOR, GITHUB_REPO_URL, LICENSE } from '../../constants'

export default function LoginPage() {
  const nav = useNavigate()
  const onFinish = async (v: { password: string }) => {
    const { data } = await http.post('/api/admin/login', v)
    localStorage.setItem('token', data.token)
    nav('/admin')
  }
  return (
    <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 360 }}>
        <Typography.Title level={3} style={{ textAlign: 'center' }}>AI Code Review</Typography.Title>
        <Form onFinish={onFinish}>
          <Form.Item name="password" rules={[{ required: true, message: '请输入管理员密码' }]}>
            <Input.Password placeholder="管理员密码" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block size="large">登录</Button>
        </Form>
      </Card>
      <div style={{ marginTop: 16, color: '#999', fontSize: 13 }}>
        作者 {AUTHOR} · {LICENSE} License ·{' '}
        <a href={GITHUB_REPO_URL} target="_blank" rel="noreferrer"><GithubOutlined /> GitHub</a>
      </div>
    </div>
  )
}
