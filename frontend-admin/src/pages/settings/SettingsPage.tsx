import React, { useCallback, useEffect, useState } from 'react'
import { Card, message, Switch, Spin, Typography, Descriptions } from 'antd'
import { adminSettingApi } from '@/api/admin-setting'
import type { SystemSetting } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

const SettingsPage: React.FC = () => {
  const [settings, setSettings] = useState<SystemSetting[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState<string | null>(null)

  const fetch = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminSettingApi.list()
      setSettings(res)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetch() }, [fetch])

  const getSetting = (key: string) => settings.find((s) => s.key === key)

  const regEnabled = getSetting('registration_enabled')
  const regOn = regEnabled?.value === 'true'

  const handleRegToggle = async (checked: boolean) => {
    setSaving('registration_enabled')
    try {
      await adminSettingApi.update('registration_enabled', checked ? 'true' : 'false')
      void message.success(`开放注册已${checked ? '开启' : '关闭'}`)
      void fetch()
    } finally {
      setSaving(null)
    }
  }

  if (loading) return <Spin />

  return (
    <div>
      <Title level={4} style={{ marginTop: 0 }}>系统设置</Title>

      <Card title="注册与访问控制" style={{ maxWidth: 640 }}>
        <Descriptions column={1} size="middle">
          <Descriptions.Item
            label={
              <div>
                <div style={{ fontWeight: 500 }}>开放注册</div>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {regEnabled?.desc ?? '是否允许用户自行注册账号'}
                </Text>
              </div>
            }
          >
            <Switch
              checked={regOn}
              loading={saving === 'registration_enabled'}
              onChange={(checked) => void handleRegToggle(checked)}
              checkedChildren="开"
              unCheckedChildren="关"
            />
            {regEnabled && (
              <Text type="secondary" style={{ marginLeft: 12, fontSize: 12 }}>
                最后修改：{dayjs(regEnabled.updatedAt).format('YYYY-MM-DD HH:mm')}
              </Text>
            )}
          </Descriptions.Item>
        </Descriptions>
        <div style={{ marginTop: 8, padding: '8px 12px', background: '#f9f9f9', borderRadius: 4, fontSize: 12, color: '#888' }}>
          关闭后 <Text code style={{ fontSize: 12 }}>/api/auth/register</Text> 将返回 1004 错误。
          已有账号不受影响。新用户需由 admin 在上方「用户管理」手动创建后分配。
        </div>
      </Card>
    </div>
  )
}

export default SettingsPage
