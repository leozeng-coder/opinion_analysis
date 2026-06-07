import React, { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  ApartmentOutlined,
  AuditOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  LogoutOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  BellOutlined,
  CloudSyncOutlined,
} from '@ant-design/icons'
import { Avatar, Breadcrumb, Layout, Menu, Space, Typography } from 'antd'
import type { MenuProps } from 'antd'
import { useAuthStore } from '@/store/auth'
import styles from './AdminLayout.module.css'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const menuItems: MenuProps['items'] = [
  { key: '/system', icon: <ThunderboltOutlined />, label: <Link to="/system">系统概览</Link> },
  { key: '/users', icon: <TeamOutlined />, label: <Link to="/users">用户管理</Link> },
  { key: '/data/platform-sync', icon: <CloudSyncOutlined />, label: <Link to="/data/platform-sync">平台同步</Link> },
  {
    key: '/ai',
    icon: <RobotOutlined />,
    label: 'AI 引擎',
    children: [
      { key: '/ai/tagger', icon: <RobotOutlined />, label: <Link to="/ai/tagger">打标任务</Link> },
      { key: '/ai/rag', icon: <DatabaseOutlined />, label: <Link to="/ai/rag">向量知识库</Link> },
    ],
  },
  {
    key: '/config',
    icon: <SettingOutlined />,
    label: '系统设置',
    children: [
      { key: '/config/system', icon: <SettingOutlined />, label: <Link to="/config/system">基础设置</Link> },
      { key: '/config/notify', icon: <BellOutlined />, label: <Link to="/config/notify">通知与告警</Link> },
      { key: '/config/crawler', icon: <CloudServerOutlined />, label: <Link to="/config/crawler">爬虫配置</Link> },
    ],
  },
  { key: '/audit', icon: <AuditOutlined />, label: <Link to="/audit">审计日志</Link> },
]

const routeLabels: Record<string, string> = {
  '/system': '系统概览',
  '/users': '用户管理',
  '/data/platform-sync': '平台同步',
  '/ai/tagger': '打标任务',
  '/ai/rag': '向量知识库',
  '/config/system': '基础设置',
  '/config/notify': '通知与告警',
  '/config/crawler': '爬虫配置',
  '/audit': '审计日志',
}

const AdminLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const { user, logout } = useAuthStore()

  const handleLogout = () => {
    logout()
    void navigate('/login')
  }

  const currentLabel = routeLabels[location.pathname] ?? ''

  const selectedKeys = [location.pathname]
  const openKeys = location.pathname.startsWith('/config') ? ['/config']
    : location.pathname.startsWith('/ai') ? ['/ai']
    : []

  return (
    <Layout className={styles.layout}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        theme="dark"
        width={220}
        className={styles.sider}
      >
        <div className={styles.logo}>
          <span className={styles.logoIcon}><ApartmentOutlined /></span>
          {!collapsed && <span>舆情分析管理</span>}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selectedKeys}
          defaultOpenKeys={openKeys}
          items={menuItems}
        />
      </Sider>

      <Layout>
        <Header className={styles.header}>
          <Breadcrumb items={[{ title: '管理后台' }, { title: currentLabel }]} />
          <Space className={styles.userTrigger} size={10} onClick={handleLogout}>
            <Avatar size="small" style={{ background: 'var(--c-blue)' }}>
              {user?.nickname?.[0] ?? user?.username?.[0] ?? 'A'}
            </Avatar>
            <Text style={{ color: 'var(--app-text)' }}>{user?.nickname ?? user?.username}</Text>
            <LogoutOutlined style={{ color: 'var(--app-text-secondary)' }} />
          </Space>
        </Header>

        <Content className={styles.content}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

export default AdminLayout
