import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Descriptions,
  InputNumber,
  Row,
  Space,
  Switch,
  Typography,
  message,
} from 'antd'
import { SettingOutlined } from '@ant-design/icons'
import { adminSettingApi } from '@/api/admin-setting'
import type { SystemSetting } from '@/types'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'
import dayjs from 'dayjs'

const { Text } = Typography

const SystemConfigPage: React.FC = () => {
  const [settings, setSettings] = useState<SystemSetting[]>([])
  const [settingsLoading, setSettingsLoading] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState<string | null>(null)
  const [thresholdInput, setThresholdInput] = useState<number>(2)

  const loadSettings = useCallback(async () => {
    setSettingsLoading(true)
    try {
      const res = await adminSettingApi.list()
      setSettings(res)
      const t = res.find((s) => s.key === 'dashboard.hot_topic_threshold')
      if (t) setThresholdInput(parseInt(t.value, 10) || 2)
    } finally {
      setSettingsLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadSettings()
  }, [loadSettings])

  const handleRegToggle = async (checked: boolean) => {
    setSettingsSaving('registration_enabled')
    try {
      await adminSettingApi.update('registration_enabled', checked ? 'true' : 'false')
      void message.success(`开放注册已${checked ? '开启' : '关闭'}`)
      void loadSettings()
    } finally {
      setSettingsSaving(null)
    }
  }

  const handleThresholdSave = async () => {
    setSettingsSaving('dashboard.hot_topic_threshold')
    try {
      await adminSettingApi.update('dashboard.hot_topic_threshold', String(thresholdInput))
      void message.success('热点话题阈值已保存')
      void loadSettings()
    } finally {
      setSettingsSaving(null)
    }
  }

  const getSetting = (key: string) => settings.find((s) => s.key === key)
  const regEnabled = getSetting('registration_enabled')
  const regOn = regEnabled?.value === 'true'
  const thresholdSetting = getSetting('dashboard.hot_topic_threshold')

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="系统配置"
        subtitle="注册控制、仪表盘参数等系统级设置"
        icon={<SettingOutlined />}
      />

      <Card bordered={false} className={ui.panelCard} title="系统设置" loading={settingsLoading}>
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={12}>
            <Card type="inner" title="注册与访问控制">
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
                    loading={settingsSaving === 'registration_enabled'}
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
                关闭后 <Text code style={{ fontSize: 12 }}>/api/auth/register</Text> 将返回 1004 错误，已有账号不受影响。
              </div>
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card type="inner" title="仪表盘配置">
              <Descriptions column={1} size="middle">
                <Descriptions.Item
                  label={
                    <div>
                      <div style={{ fontWeight: 500 }}>热点话题阈值</div>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {thresholdSetting?.desc ?? 'AI 标签在文章中出现 ≥ 该值视为热点话题'}
                      </Text>
                    </div>
                  }
                >
                  <Space wrap>
                    <InputNumber
                      min={1}
                      max={999}
                      value={thresholdInput}
                      onChange={(v) => setThresholdInput(v ?? 2)}
                      style={{ width: 140 }}
                      addonAfter="篇"
                    />
                    <Button
                      type="primary"
                      size="small"
                      loading={settingsSaving === 'dashboard.hot_topic_threshold'}
                      onClick={() => void handleThresholdSave()}
                    >
                      保存
                    </Button>
                    {thresholdSetting && (
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        最后修改：{dayjs(thresholdSetting.updatedAt).format('YYYY-MM-DD HH:mm')}
                      </Text>
                    )}
                  </Space>
                </Descriptions.Item>
              </Descriptions>
              <div style={{ marginTop: 8, padding: '8px 12px', background: '#f9f9f9', borderRadius: 4, fontSize: 12, color: '#888' }}>
                仪表盘「热点话题」统计 AI 标签出现次数 ≥ 阈值的标签数量；阈值越高，话题越聚焦。
              </div>
            </Card>
          </Col>
        </Row>
      </Card>
    </div>
  )
}

export default SystemConfigPage
