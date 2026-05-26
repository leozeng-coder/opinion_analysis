import React, { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import {
  Layout, Menu, Avatar, Dropdown, Badge, Space, Typography,
} from 'antd'
import {
  DashboardOutlined, FileTextOutlined, FireOutlined,
  BellOutlined, UserOutlined, LogoutOutlined, BarChartOutlined, CloudSyncOutlined,
  CommentOutlined, RadarChartOutlined, AppstoreOutlined,
} from '@ant-design/icons'
import { useAuthStore } from '@/store/auth'
import DraggableAssistantLauncher from '@/components/layout/DraggableAssistantLauncher'
import styles from './AppLayout.module.css'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '概览仪表盘' },
  { key: '/opinion', icon: <FileTextOutlined />, label: '舆情数据' },
  { key: '/topics', icon: <FireOutlined />, label: '热点话题' },
  { key: '/alerts', icon: <BellOutlined />, label: '预警中心' },
  { key: '/stats', icon: <BarChartOutlined />, label: '统计分析' },
  { key: '/crawler', icon: <CloudSyncOutlined />, label: '爬虫调度' },
  { key: '/platform', icon: <AppstoreOutlined />, label: '平台数据' },
  { key: '/assistant', icon: <CommentOutlined />, label: '智能助手' },
]

const AppLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
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
    <Layout className={styles.layout}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        className={styles.sider}
        width={220}
      >
        <div className={styles.logo}>
          <span className={styles.logoIcon}><RadarChartOutlined /></span>
          {!collapsed && <span>舆情分析系统</span>}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>

      <Layout className={collapsed ? styles.layoutMainCollapsed : styles.layoutMain}>
        <Header className={styles.header}>
          <Space size={12}>
            <Badge count={0} size="small" showZero={false}>
              <div className={styles.headerAction}>
                <BellOutlined style={{ fontSize: 17 }} />
              </div>
            </Badge>
            <Dropdown menu={{ items: userMenuItems, onClick: handleUserMenu }}>
              <Space className={styles.userTrigger} size={10}>
                <Avatar icon={<UserOutlined />} size="small"
                  style={{ background: 'var(--c-blue)' }} />
                <Text style={{ color: 'var(--app-text)' }}>
                  {user?.nickname || user?.username}
                </Text>
              </Space>
            </Dropdown>
          </Space>
        </Header>

        <Content className={styles.content}>
          <Outlet />
        </Content>
      </Layout>

      {location.pathname !== '/assistant' && (
        <DraggableAssistantLauncher onOpen={() => navigate('/assistant')} />
      )}
    </Layout>
  )
}

export default AppLayout
