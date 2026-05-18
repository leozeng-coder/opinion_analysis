import React, { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import {
  Layout, Menu, Avatar, Dropdown, theme, Badge, Space, Typography,
  FloatButton,
} from 'antd'
import {
  DashboardOutlined, FileTextOutlined, FireOutlined,
  BellOutlined, UserOutlined, LogoutOutlined, BarChartOutlined, CloudSyncOutlined,
  CommentOutlined,
} from '@ant-design/icons'
import { useAuthStore } from '@/store/auth'
import AiChatDrawer from '@/components/assistant/AiChatDrawer'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '概览仪表盘' },
  { key: '/opinion', icon: <FileTextOutlined />, label: '舆情数据' },
  { key: '/topics', icon: <FireOutlined />, label: '热点话题' },
  { key: '/alerts', icon: <BellOutlined />, label: '预警中心' },
  { key: '/stats', icon: <BarChartOutlined />, label: '统计分析' },
  { key: '/crawler', icon: <CloudSyncOutlined />, label: '爬虫调度' },
]

const AppLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const [aiDrawerOpen, setAiDrawerOpen] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { token: { colorBgContainer, borderRadiusLG } } = theme.useToken()
  const { user, logout } = useAuthStore()

  const userMenuItems = [
    { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
    { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
  ]

  const handleUserMenu = ({ key }: { key: string }) => {
    if (key === 'logout') {
      logout()
      navigate('/login')
    }
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        style={{ background: '#001529' }}
      >
        <div style={{
          height: 64, display: 'flex', alignItems: 'center',
          justifyContent: 'center', color: '#fff', fontSize: collapsed ? 14 : 18,
          fontWeight: 'bold', overflow: 'hidden', whiteSpace: 'nowrap',
        }}>
          {collapsed ? '舆' : '舆情分析系统'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>

      <Layout>
        <Header style={{
          padding: '0 24px', background: colorBgContainer,
          display: 'flex', alignItems: 'center', justifyContent: 'flex-end',
          borderBottom: '1px solid #f0f0f0',
        }}>
          <Space size={16}>
            <Badge count={3} size="small">
              <BellOutlined style={{ fontSize: 18, cursor: 'pointer' }} />
            </Badge>
            <Dropdown menu={{ items: userMenuItems, onClick: handleUserMenu }}>
              <Space style={{ cursor: 'pointer' }}>
                <Avatar icon={<UserOutlined />} size="small" />
                <Text>{user?.nickname || user?.username}</Text>
              </Space>
            </Dropdown>
          </Space>
        </Header>

        <Content style={{ margin: '24px', background: colorBgContainer, borderRadius: borderRadiusLG, padding: 24, overflow: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>

      <FloatButton
        type="primary"
        icon={<CommentOutlined />}
        tooltip="智能助手"
        onClick={() => setAiDrawerOpen(true)}
        style={{ right: 24, bottom: 24 }}
      />
      <AiChatDrawer open={aiDrawerOpen} onClose={() => setAiDrawerOpen(false)} />
    </Layout>
  )
}

export default AppLayout
