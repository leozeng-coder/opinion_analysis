import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Progress,
  Space,
  Table,
  Typography,
  message,
} from 'antd'
import { SyncOutlined } from '@ant-design/icons'
import { platformSyncApi, type PlatformInfo, type PlatformSyncProgress } from '@/api/crawler'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'

const { Text } = Typography

const PlatformSyncPage: React.FC = () => {
  const [platforms, setPlatforms] = useState<PlatformInfo[]>([])
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([])
  const [syncProgress, setSyncProgress] = useState<Record<string, PlatformSyncProgress>>({})
  const [syncing, setSyncing] = useState(false)
  const [loading, setLoading] = useState(false)
  const [progressInterval, setProgressInterval] = useState<ReturnType<typeof setInterval> | null>(null)

  const loadPlatforms = useCallback(async () => {
    setLoading(true)
    try {
      setPlatforms(await platformSyncApi.getPlatformList())
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  const loadSyncProgress = useCallback(async () => {
    try {
      const progress = await platformSyncApi.getProgress()
      if (Array.isArray(progress)) {
        const map: Record<string, PlatformSyncProgress> = {}
        progress.forEach((p) => { map[p.platform] = p })
        setSyncProgress(map)
      }
    } catch { /* ignore */ }
  }, [])

  useEffect(() => { void loadPlatforms() }, [loadPlatforms])

  useEffect(() => {
    return () => { if (progressInterval) clearInterval(progressInterval) }
  }, [progressInterval])

  const startProgressPolling = useCallback(() => {
    if (progressInterval) return
    const interval = setInterval(() => { void loadSyncProgress() }, 1000)
    setProgressInterval(interval)
  }, [progressInterval, loadSyncProgress])

  const stopProgressPolling = useCallback(() => {
    if (progressInterval) { clearInterval(progressInterval); setProgressInterval(null) }
  }, [progressInterval])

  const handleSelectAll = (checked: boolean) =>
    setSelectedPlatforms(checked ? platforms.map((p) => p.code) : [])

  const handleSelectPlatform = (code: string, checked: boolean) =>
    setSelectedPlatforms((prev) => checked ? [...prev, code] : prev.filter((p) => p !== code))

  const handleSyncSelected = async () => {
    if (selectedPlatforms.length === 0) { message.warning('请至少选择一个平台'); return }
    setSyncing(true); startProgressPolling()
    try {
      const results = await platformSyncApi.syncPlatforms(selectedPlatforms)
      await new Promise((r) => setTimeout(r, 1000))
      const totalNew = Object.values(results).reduce((s, r) => s + r.newCount, 0)
      const failed = Object.values(results).filter((r) => r.status === 'failed').length
      failed > 0
        ? message.warning(`同步完成：新增 ${totalNew} 条，${failed} 个平台失败`)
        : message.success(`同步完成：新增 ${totalNew} 条`)
      void loadPlatforms()
    } catch { message.error('同步失败') }
    finally { setSyncing(false); setTimeout(() => { stopProgressPolling(); setSyncProgress({}) }, 2000) }
  }

  const handleSyncAll = async () => {
    setSyncing(true); startProgressPolling()
    try {
      const results = await platformSyncApi.syncAll()
      await new Promise((r) => setTimeout(r, 1000))
      const totalNew = Object.values(results).reduce((s, r) => s + r.newCount, 0)
      message.success(`批量同步完成：共新增 ${totalNew} 条`)
      void loadPlatforms()
    } catch { message.error('批量同步失败') }
    finally { setSyncing(false); setTimeout(() => { stopProgressPolling(); setSyncProgress({}) }, 2000) }
  }

  const getProgressPercent = (platform: string) => {
    const p = syncProgress[platform]
    if (!p || p.totalCount === 0) return 0
    return Math.round((p.processedCount / p.totalCount) * 100)
  }

  const getProgressStatus = (platform: string): 'success' | 'exception' | 'active' | 'normal' => {
    const p = syncProgress[platform]
    if (!p) return 'normal'
    if (p.status === 'completed') return 'success'
    if (p.status === 'failed') return 'exception'
    if (p.status === 'running') return 'active'
    return 'normal'
  }

  const columns = [
    {
      title: (
        <Checkbox
          checked={selectedPlatforms.length === platforms.length && platforms.length > 0}
          indeterminate={selectedPlatforms.length > 0 && selectedPlatforms.length < platforms.length}
          onChange={(e) => handleSelectAll(e.target.checked)}
        >
          平台
        </Checkbox>
      ),
      dataIndex: 'name',
      render: (name: string, record: PlatformInfo) => (
        <Checkbox
          checked={selectedPlatforms.includes(record.code)}
          onChange={(e) => handleSelectPlatform(record.code, e.target.checked)}
        >
          {name}
        </Checkbox>
      ),
    },
    { title: '数据表', dataIndex: 'table' },
    {
      title: '最后同步时间',
      dataIndex: 'lastSyncTime',
      render: (time: string) => {
        if (!time) return <Text type="secondary">从未同步</Text>
        const diff = Date.now() - new Date(time).getTime()
        const min = Math.floor(diff / 60000)
        if (min < 1) return '刚刚'
        if (min < 60) return `${min} 分钟前`
        if (min < 1440) return `${Math.floor(min / 60)} 小时前`
        return new Date(time).toLocaleString('zh-CN')
      },
    },
    {
      title: '同步进度',
      render: (_: unknown, record: PlatformInfo) => {
        const p = syncProgress[record.code]
        if (!p || p.status === 'pending') return <Text type="secondary">-</Text>
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Progress percent={getProgressPercent(record.code)} status={getProgressStatus(record.code)} size="small" />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {p.status === 'running' && `处理中: ${p.processedCount}/${p.totalCount}`}
              {p.status === 'completed' && `完成: 新增 ${p.newCount}, 跳过 ${p.skippedCount}`}
              {p.status === 'failed' && `失败: ${p.errorMessage}`}
            </Text>
          </Space>
        )
      },
    },
  ]

  return (
    <div className={ui.pageShell}>
      <PageHeader
        title="平台同步"
        subtitle="手动触发各平台数据同步，查看同步进度"
        icon={<SyncOutlined />}
      />
      <Card
        bordered={false}
        className={ui.panelCard}
        extra={
          <Space>
            <Button
              type="primary"
              icon={<SyncOutlined />}
              onClick={() => void handleSyncSelected()}
              loading={syncing}
              disabled={selectedPlatforms.length === 0}
            >
              同步选中平台
            </Button>
            <Button icon={<SyncOutlined />} onClick={() => void handleSyncAll()} loading={syncing}>
              同步所有平台
            </Button>
          </Space>
        }
      >
        <Alert
          message="自动同步说明"
          description="MediaCrawler 爬虫完成后会自动触发数据同步，无需手动配置。此处提供手动同步功能用于紧急情况，支持多选平台批量同步并实时查看进度。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Table columns={columns} dataSource={platforms} rowKey="code" pagination={false} loading={loading} />
      </Card>
    </div>
  )
}

export default PlatformSyncPage
