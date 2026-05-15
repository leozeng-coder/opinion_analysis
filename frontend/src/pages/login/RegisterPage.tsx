import React from 'react'
import { Form, Input, Button, Card, message, Typography } from 'antd'
import { UserOutlined, LockOutlined, MailOutlined, IdcardOutlined } from '@ant-design/icons'
import { Link, useNavigate } from 'react-router-dom'
import { authApi } from '@/api/auth'

const { Title } = Typography

const RegisterPage: React.FC = () => {
  const navigate = useNavigate()
  const [loading, setLoading] = React.useState(false)

  const onFinish = async (values: {
    username: string
    password: string
    email: string
    nickname?: string
  }) => {
    setLoading(true)
    try {
      await authApi.register({
        username: values.username,
        password: values.password,
        email: values.email,
        nickname: values.nickname?.trim() || undefined,
      })
      message.success('注册成功，请登录')
      navigate('/login', { replace: true })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      height: '100vh', display: 'flex', alignItems: 'center',
      justifyContent: 'center', background: '#f0f2f5',
    }}>
      <Card style={{ width: 380, boxShadow: '0 4px 24px rgba(0,0,0,.08)' }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={3} style={{ margin: 0 }}>注册账号</Title>
          <Typography.Text type="secondary">舆情分析系统</Typography.Text>
        </div>
        <Form onFinish={onFinish} size="large" autoComplete="off" layout="vertical" style={{ marginBottom: 0 }}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[
              { required: true, message: '请输入用户名' },
              { min: 3, message: '至少 3 个字符' },
              { max: 32, message: '最多 32 个字符' },
            ]}
          >
            <Input prefix={<UserOutlined />} placeholder="3–32 个字符" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 6, message: '至少 6 个字符' },
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="至少 6 个字符" />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '邮箱格式不正确' },
            ]}
          >
            <Input prefix={<MailOutlined />} placeholder="name@example.com" />
          </Form.Item>
          <Form.Item name="nickname" label="昵称（可选）">
            <Input prefix={<IdcardOutlined />} placeholder="显示名称" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>
              注册
            </Button>
          </Form.Item>
        </Form>
        <div style={{ textAlign: 'center' }}>
          <Link to="/login">已有账号？去登录</Link>
        </div>
      </Card>
    </div>
  )
}

export default RegisterPage
