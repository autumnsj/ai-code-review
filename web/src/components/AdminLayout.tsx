import { Layout, Menu, Button, theme } from 'antd'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  DashboardOutlined,
  BranchesOutlined,
  AuditOutlined,
  TeamOutlined,
  UnorderedListOutlined,
  SettingOutlined,
  LogoutOutlined,
} from '@ant-design/icons'

const { Sider, Content, Header } = Layout

const items = [
  { key: '/admin/dashboard', icon: <DashboardOutlined />, label: <Link to="/admin/dashboard">概览</Link> },
  { key: '/admin/repos', icon: <BranchesOutlined />, label: <Link to="/admin/repos">仓库</Link> },
  { key: '/admin/reviews', icon: <AuditOutlined />, label: <Link to="/admin/reviews">审查记录</Link> },
  { key: '/admin/stats', icon: <TeamOutlined />, label: <Link to="/admin/stats/authors">作者排行</Link> },
  { key: '/admin/jobs', icon: <UnorderedListOutlined />, label: <Link to="/admin/jobs">任务队列</Link> },
  { key: '/admin/settings', icon: <SettingOutlined />, label: <Link to="/admin/settings">设置</Link> },
]

export default function AdminLayout() {
  const loc = useLocation()
  const nav = useNavigate()
  const { token } = theme.useToken()
  const selectedKey = '/' + loc.pathname.split('/').slice(1, 3).join('/')

  const logout = () => {
    localStorage.removeItem('token')
    nav('/login')
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider theme="light" width={200}>
        <div style={{ height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, fontSize: 16 }}>
          AI Code Review
        </div>
        <Menu mode="inline" selectedKeys={[selectedKey]} items={items} />
      </Sider>
      <Layout>
        <Header style={{ background: token.colorBgContainer, padding: '0 24px', display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Button type="text" icon={<LogoutOutlined />} onClick={logout}>退出登录</Button>
        </Header>
        <Content style={{ margin: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
