import { Layout } from 'antd'
import { GithubOutlined } from '@ant-design/icons'
import { AUTHOR, GITHUB_REPO_URL, LICENSE, PROJECT_NAME } from '../constants'

export default function Footer() {
  return (
    <Layout.Footer style={{ textAlign: 'center', color: '#999', fontSize: 13, padding: '12px 24px' }}>
      {PROJECT_NAME} · 作者 {AUTHOR} · {LICENSE} License ·{' '}
      <a href={GITHUB_REPO_URL} target="_blank" rel="noreferrer">
        <GithubOutlined /> GitHub
      </a>
    </Layout.Footer>
  )
}
