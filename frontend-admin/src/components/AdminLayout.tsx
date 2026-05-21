import React, { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  ApartmentOutlined,
  AuditOutlined,
  DatabaseOutlined,
  LogoutOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  ApiOutlined,
  BellOutlined,
} from '@ant-design/icons'
import { Avatar, Breadcrumb, Layout, Menu, Space, Typography } from 'antd'
import type { MenuProps } from 'antd'
import { useAuthStore } from '@/store/auth'
import styles from './AdminLayout.module.css'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const menuItems: MenuProps['items'] = [
  { key: '/system', icon: <ThunderboltOutlined />, label: <Link to="/system">系统状态</Link> },
  { key: '/users', icon: <TeamOutlined />, label: <Link to="/users">用户管理</Link> },
  {
    key: '/config',
    icon: <SettingOutlined />,
    label: '配置中心',
    children: [
      { key: '/config/system', icon: <SettingOutlined />, label: <Link to="/config/system">系统配置</Link> },
      { key: '/config/ai', icon: <RobotOutlined />, label: <Link to="/config/ai">AI配置</Link> },
      { key: '/config/crawler', icon: <BellOutlined />, label: <Link to="/config/crawler">爬虫配置</Link> },
    ],
  },
  { key: '/tasks', icon: <RobotOutlined />, label: <Link to="/tasks">任务管理</Link> },
  { key: '/rag-kb', icon: <DatabaseOutlined />, label: <Link to="/rag-kb">知识库管理</Link> },
  { key: '/datasources', icon: <ApiOutlined />, label: <Link to="/datasources">数据源</Link> },
  { key: '/audit', icon: <AuditOutlined />, label: <Link to="/audit">审计日志</Link> },
]

const routeLabels: Record<string, string> = {
  '/system': '系统状态',
  '/users': '用户管理',
  '/config/system': '系统配置',
  '/config/ai': 'AI配置',
  '/config/crawler': '爬虫配置',
  '/tasks': '任务管理',
  '/rag-kb': '知识库管理',
  '/datasources': '数据源管理',
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

  // 确定当前选中的菜单项和展开的子菜单
  const selectedKeys = [location.pathname]
  const openKeys = location.pathname.startsWith('/config') ? ['/config'] : []

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
