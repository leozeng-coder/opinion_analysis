import React, { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Form, Input, message, Typography } from 'antd'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { authApi } from '@/api/auth'
import { useAuthStore } from '@/store/auth'
import type { User } from '@/types'

const { Title, Text } = Typography

const LoginPage: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const { setAuth, token, user } = useAuthStore()
  const navigate = useNavigate()
  const location = useLocation()
  const errorMsg = (location.state as { error?: string } | null)?.error

  useEffect(() => {
    if (token && user?.role === 'admin') {
      void navigate('/', { replace: true })
    }
  }, [token, user, navigate])

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      const res = await authApi.login(values.username, values.password)
      if ((res.user as User).role !== 'admin') {
        void message.error('该账号没有 admin 权限，无法访问管理后台')
        return
      }
      setAuth(res.token, res.user as User)
      void navigate('/', { replace: true })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: '#f0f2f5',
    }}>
      <Card style={{ width: 380, boxShadow: '0 2px 16px rgba(0,0,0,0.1)' }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={3} style={{ marginBottom: 4 }}>舆情分析系统 管理后台</Title>
          <Text type="secondary">需要 admin 角色账号</Text>
        </div>
        {errorMsg && <Alert type="error" message={errorMsg} style={{ marginBottom: 16 }} />}
        <Form layout="vertical" onFinish={(v) => void onFinish(v as { username: string; password: string })}>
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" size="large" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block size="large" loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  )
}

export default LoginPage
