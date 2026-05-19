import React, { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  ApartmentOutlined,
  AuditOutlined,
  CloudSyncOutlined,
  DatabaseOutlined,
  LogoutOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { Avatar, Breadcrumb, Layout, Menu, Space, Typography } from 'antd'
import type { MenuProps } from 'antd'
import { useAuthStore } from '@/store/auth'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const menuItems: MenuProps['items'] = [
  { key: '/system', icon: <ThunderboltOutlined />, label: <Link to="/system">系统状态</Link> },
  { key: '/users', icon: <TeamOutlined />, label: <Link to="/users">用户管理</Link> },
  { key: '/config', icon: <SettingOutlined />, label: <Link to="/config">系统配置</Link> },
  { key: '/crawler', icon: <CloudSyncOutlined />, label: <Link to="/crawler">爬虫运维</Link> },
  { key: '/tasks', icon: <RobotOutlined />, label: <Link to="/tasks">任务管理</Link> },
  { key: '/rag-kb', icon: <DatabaseOutlined />, label: <Link to="/rag-kb">知识库管理</Link> },
  { key: '/datasources', icon: <DatabaseOutlined />, label: <Link to="/datasources">数据源</Link> },
  { key: '/audit', icon: <AuditOutlined />, label: <Link to="/audit">审计日志</Link> },
]

const routeLabels: Record<string, string> = {
  '/system': '系统状态',
  '/users': '用户管理',
  '/config': '系统配置',
  '/crawler': '爬虫运维',
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

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        theme="dark"
        width={220}
        style={{ position: 'sticky', top: 0, height: '100vh', overflow: 'auto' }}
      >
        <div style={{ height: 56, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '0 16px' }}>
          <Space>
            <ApartmentOutlined style={{ color: '#fff', fontSize: 20 }} />
            {!collapsed && <Text style={{ color: '#fff', fontWeight: 600, fontSize: 15 }}>舆情分析管理</Text>}
          </Space>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
        />
      </Sider>

      <Layout>
        <Header
          style={{
            background: '#fff',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #f0f0f0',
            height: 56,
          }}
        >
          <Breadcrumb items={[{ title: '管理后台' }, { title: currentLabel }]} />
          <Space style={{ cursor: 'pointer' }} onClick={handleLogout}>
            <Avatar style={{ backgroundColor: '#1677ff' }}>
              {user?.nickname?.[0] ?? user?.username?.[0] ?? 'A'}
            </Avatar>
            <Text>{user?.nickname ?? user?.username}</Text>
            <LogoutOutlined />
          </Space>
        </Header>

        <Content style={{ margin: 24, minHeight: 360 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

export default AdminLayout
