import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Progress,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  BellOutlined,
  KeyOutlined,
  SaveOutlined,
  SettingOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { adminCrawlerApi, type CrawlerConfigResponse } from '@/api/admin-crawler'
import { platformSyncApi, type PlatformInfo, type PlatformSyncProgress } from '@/api/crawler'
import PageHeader from '@/components/common/PageHeader'
import ui from '@/styles/page.module.css'

const { Text } = Typography

const PLATFORMS = [
  { code: 'xhs', name: '小红书', color: '#ff2442' },
  { code: 'dy', name: '抖音', color: '#161823' },
  { code: 'ks', name: '快手', color: '#ff5500' },
  { code: 'bili', name: 'B站', color: '#00a1d6' },
  { code: 'wb', name: '微博', color: '#e6162d' },
  { code: 'tieba', name: '贴吧', color: '#2468f2' },
  { code: 'zhihu', name: '知乎', color: '#0066ff' },
]

const XHS_SORT_OPTIONS = [
  { value: 'time_descending', label: '最新发布' },
  { value: 'popularity_descending', label: '最受欢迎' },
]
const WB_SEARCH_OPTIONS = [
  { value: 'real_time', label: '实时' },
  { value: 'hot', label: '热门' },
  { value: 'comprehensive', label: '综合' },
]
const DY_SORT_OPTIONS = [
  { value: 0, label: '综合排序' },
  { value: 1, label: '最多点赞' },
  { value: 2, label: '最新发布' },
]
const ZHIHU_SORT_OPTIONS = [
  { value: 'created_time', label: '最新' },
  { value: 'default', label: '默认' },
]
const ZHIHU_TIME_OPTIONS = [
  { value: '', label: '不限时间' },
  { value: 'a_day', label: '一天内' },
  { value: 'a_week', label: '一周内' },
  { value: 'a_month', label: '一月内' },
  { value: 'three_months', label: '三月内' },
  { value: 'half_a_year', label: '半年内' },
  { value: 'a_year', label: '一年内' },
]
const IP_PROXY_PROVIDERS = [
  { value: 'kuaidaili', label: '快代理 (KDL)' },
  { value: 'wandou', label: '豌豆代理' },
]

interface CrawlerFormValues {
  maxNotesCount: number; maxConcurrency: number; sleepSecMin: number; sleepSecMax: number
  enableIPProxy: boolean; ipProxyPoolCount: number; ipProxyProvider: string
  proxyKdlSecretId: string; proxyKdlSignature: string
  proxyKdlUsername: string; proxyKdlPassword: string; proxyWandouAppKey: string
  xhsSortType: string; weiboSearchType: string; dySortType: number
  zhihuSort: string; zhihuSearchTime: string
}

const CrawlerConfigPage: React.FC = () => {
  const [platforms, setPlatforms] = useState<PlatformInfo[]>([])
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([])
  const [syncProgress, setSyncProgress] = useState<{ [key: string]: PlatformSyncProgress }>({})
  const [syncing, setSyncing] = useState(false)
  const [progressInterval, setProgressInterval] = useState<ReturnType<typeof setInterval> | null>(null)

  const [crawlerForm] = Form.useForm<CrawlerFormValues>()
  const enableIPProxy = Form.useWatch('enableIPProxy', crawlerForm) ?? false
  const ipProxyProvider = Form.useWatch('ipProxyProvider', crawlerForm) ?? 'kuaidaili'
  const [crawlerLoading, setCrawlerLoading] = useState(false)
  const [crawlerSaving, setCrawlerSaving] = useState(false)
  const [cookies, setCookies] = useState<CrawlerConfigResponse['cookies']>({})

  const [cookieModal, setCookieModal] = useState<{ open: boolean; platform: string; name: string }>({
    open: false, platform: '', name: '',
  })
  const [cookieInput, setCookieInput] = useState('')
  const [cookieSaving, setCookieSaving] = useState(false)

  const loadPlatforms = useCallback(async () => {
    try {
      const list = await platformSyncApi.getPlatformList()
      setPlatforms(list)
    } catch { /* ignore */ }
  }, [])

  const loadSyncProgress = useCallback(async () => {
    try {
      const progress = await platformSyncApi.getProgress()
      if (Array.isArray(progress)) {
        const map: { [key: string]: PlatformSyncProgress } = {}
        progress.forEach((p) => { map[p.platform] = p })
        setSyncProgress(map)
      }
    } catch { /* ignore */ }
  }, [])

  const loadCrawlerConfig = useCallback(async () => {
    setCrawlerLoading(true)
    try {
      const cfg = await adminCrawlerApi.getConfig()
      crawlerForm.setFieldsValue({
        maxNotesCount: cfg.maxNotesCount, maxConcurrency: cfg.maxConcurrency,
        sleepSecMin: cfg.sleepSecMin, sleepSecMax: cfg.sleepSecMax,
        enableIPProxy: cfg.enableIPProxy, ipProxyPoolCount: cfg.ipProxyPoolCount,
        ipProxyProvider: cfg.ipProxyProvider || 'kuaidaili',
        proxyKdlSecretId: cfg.proxyKdlSecretId, proxyKdlSignature: cfg.proxyKdlSignature,
        proxyKdlUsername: cfg.proxyKdlUsername, proxyKdlPassword: cfg.proxyKdlPassword,
        proxyWandouAppKey: cfg.proxyWandouAppKey,
        xhsSortType: cfg.xhsSortType, weiboSearchType: cfg.weiboSearchType,
        dySortType: cfg.dySortType, zhihuSort: cfg.zhihuSort, zhihuSearchTime: cfg.zhihuSearchTime,
      })
      setCookies(cfg.cookies ?? {})
    } catch { message.error('加载爬虫配置失败') }
    finally { setCrawlerLoading(false) }
  }, [crawlerForm])

  useEffect(() => {
    void loadPlatforms()
    void loadCrawlerConfig()
  }, [loadPlatforms, loadCrawlerConfig])

  const startProgressPolling = useCallback(() => {
    if (progressInterval) return
    const interval = setInterval(() => { void loadSyncProgress() }, 1000)
    setProgressInterval(interval)
  }, [progressInterval, loadSyncProgress])

  const stopProgressPolling = useCallback(() => {
    if (progressInterval) { clearInterval(progressInterval); setProgressInterval(null) }
  }, [progressInterval])

  useEffect(() => {
    return () => { if (progressInterval) clearInterval(progressInterval) }
  }, [progressInterval])

  const handleSaveCrawlerParams = async (values: CrawlerFormValues) => {
    setCrawlerSaving(true)
    try {
      const updated = await adminCrawlerApi.updateConfig(values)
      message.success('爬虫参数已保存')
      setCookies(updated.cookies ?? {})
    } catch { message.error('保存失败') }
    finally { setCrawlerSaving(false) }
  }

  const handleOpenCookieModal = (code: string, name: string) => {
    setCookieModal({ open: true, platform: code, name })
    setCookieInput('')
  }

  const handleSaveCookie = async () => {
    if (!cookieInput.trim()) { message.warning('Cookie 不能为空'); return }
    setCookieSaving(true)
    try {
      const updated = await adminCrawlerApi.updateConfig({ cookies: { [cookieModal.platform]: cookieInput.trim() } })
      message.success(`${cookieModal.name} Cookie 已更新`)
      setCookies(updated.cookies ?? {})
      setCookieModal({ open: false, platform: '', name: '' })
    } catch { message.error('更新失败') }
    finally { setCookieSaving(false) }
  }

  const handleClearCookie = async (code: string, name: string) => {
    try {
      const updated = await adminCrawlerApi.updateConfig({ cookies: { [code]: '' } })
      message.success(`${name} Cookie 已清除`)
      setCookies(updated.cookies ?? {})
    } catch { message.error('清除失败') }
  }

  const handleSyncSelected = async () => {
    if (selectedPlatforms.length === 0) { message.warning('请至少选择一个平台'); return }
    setSyncing(true); startProgressPolling()
    try {
      const results = await platformSyncApi.syncPlatforms(selectedPlatforms)
      await new Promise(resolve => setTimeout(resolve, 1000))
      const totalNew = Object.values(results).reduce((s, r) => s + r.newCount, 0)
      const failed = Object.values(results).filter(r => r.status === 'failed').length
      if (failed > 0) message.warning(`同步完成：新增 ${totalNew} 条，${failed} 个平台失败`)
      else message.success(`同步完成：新增 ${totalNew} 条数据`)
      void loadPlatforms()
    } catch { message.error('同步失败') }
    finally {
      setSyncing(false)
      setTimeout(() => { stopProgressPolling(); setSyncProgress({}) }, 2000)
    }
  }

  const handleSyncAll = async () => {
    setSyncing(true); startProgressPolling()
    try {
      const results = await platformSyncApi.syncAll()
      await new Promise(resolve => setTimeout(resolve, 1000))
      const totalNew = Object.values(results).reduce((s, r) => s + r.newCount, 0)
      message.success(`批量同步完成：共新增 ${totalNew} 条数据`)
      void loadPlatforms()
    } catch { message.error('批量同步失败') }
    finally {
      setSyncing(false)
      setTimeout(() => { stopProgressPolling(); setSyncProgress({}) }, 2000)
    }
  }

  const handleSelectAll = (checked: boolean) => {
    setSelectedPlatforms(checked ? platforms.map(p => p.code) : [])
  }
  const handleSelectPlatform = (code: string, checked: boolean) => {
    setSelectedPlatforms(checked ? [...selectedPlatforms, code] : selectedPlatforms.filter(p => p !== code))
  }
  const getProgressPercent = (platform: string) => {
    const p = syncProgress[platform]
    return (!p || p.totalCount === 0) ? 0 : Math.round((p.processedCount / p.totalCount) * 100)
  }
  const getProgressStatus = (platform: string): 'success' | 'exception' | 'active' | 'normal' => {
    const p = syncProgress[platform]
    if (!p) return 'normal'
    if (p.status === 'completed') return 'success'
    if (p.status === 'failed') return 'exception'
    if (p.status === 'running') return 'active'
    return 'normal'
  }

  const syncColumns = [
    {
      title: (
        <Checkbox
          checked={selectedPlatforms.length === platforms.length && platforms.length > 0}
          indeterminate={selectedPlatforms.length > 0 && selectedPlatforms.length < platforms.length}
          onChange={(e) => handleSelectAll(e.target.checked)}
        >平台</Checkbox>
      ),
      dataIndex: 'name', key: 'name',
      render: (name: string, record: PlatformInfo) => (
        <Checkbox checked={selectedPlatforms.includes(record.code)}
          onChange={(e) => handleSelectPlatform(record.code, e.target.checked)}>{name}</Checkbox>
      ),
    },
    { title: '数据表', dataIndex: 'table', key: 'table' },
    {
      title: '最后同步时间', dataIndex: 'lastSyncTime', key: 'lastSyncTime',
      render: (time: string) => {
        if (!time) return <Text type="secondary">从未同步</Text>
        const diff = Date.now() - new Date(time).getTime()
        const mins = Math.floor(diff / 60000)
        if (mins < 1) return '刚刚'
        if (mins < 60) return `${mins} 分钟前`
        if (mins < 1440) return `${Math.floor(mins / 60)} 小时前`
        return new Date(time).toLocaleString('zh-CN')
      },
    },
    {
      title: '同步进度', key: 'progress',
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
    <div className={ui.page}>
      <PageHeader title="爬虫配置" />

      <Row gutter={[16, 16]}>
        {/* 平台数据同步 */}
        <Col span={24}>
          <Card className={ui.panelCard} title="🔄 平台数据同步"
            extra={<Space>
              <Button type="primary" icon={<SyncOutlined />} onClick={handleSyncSelected}
                loading={syncing} disabled={selectedPlatforms.length === 0}>同步选中平台</Button>
              <Button icon={<SyncOutlined />} onClick={handleSyncAll} loading={syncing}>同步所有平台</Button>
            </Space>}
          >
            <Alert className={ui.infoBanner} message="自动同步说明"
              description="MediaCrawler 爬虫完成后会自动触发数据同步。此处提供手动同步功能用于紧急情况，支持多平台批量同步。"
              type="info" showIcon style={{ marginBottom: 16 }} />
            <Table columns={syncColumns} dataSource={platforms} rowKey="code"
              pagination={false} />
          </Card>
        </Col>

        {/* 爬虫参数配置 */}
        <Col span={24}>
          <Card className={ui.panelCard} title={<><SettingOutlined style={{ marginRight: 6 }} />爬虫参数配置</>}>
            <Spin spinning={crawlerLoading}>
              <Form form={crawlerForm} layout="vertical" onFinish={handleSaveCrawlerParams}>

                <Divider orientation="left" plain style={{ marginTop: 0, color: '#888', fontSize: 12 }}>爬取行为</Divider>
                <Row gutter={16}>
                  <Col span={6}>
                    <Form.Item label="最大爬取数量" name="maxNotesCount" tooltip="每次任务最多爬取的内容条数">
                      <InputNumber min={1} max={500} style={{ width: '100%' }} addonAfter="条" />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="最大并发数" name="maxConcurrency" tooltip="同时运行的爬取协程数，建议 1~5">
                      <InputNumber min={1} max={10} style={{ width: '100%' }} addonAfter="个" />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="请求间隔最小值" name="sleepSecMin">
                      <InputNumber min={0} max={60} style={{ width: '100%' }} addonAfter="秒" />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="请求间隔最大值" name="sleepSecMax">
                      <InputNumber min={0} max={120} style={{ width: '100%' }} addonAfter="秒" />
                    </Form.Item>
                  </Col>
                </Row>

                {/* IP 代理 */}
                <Divider orientation="left" plain style={{ color: '#888', fontSize: 12 }}>IP 代理池</Divider>
                <Row gutter={16}>
                  <Col span={4}>
                    <Form.Item label="启用 IP 代理" name="enableIPProxy" valuePropName="checked">
                      <Switch checkedChildren="开启" unCheckedChildren="关闭" />
                    </Form.Item>
                  </Col>
                  {enableIPProxy && (<>
                    <Col span={6}>
                      <Form.Item label="代理服务商" name="ipProxyProvider">
                        <Select options={IP_PROXY_PROVIDERS} />
                      </Form.Item>
                    </Col>
                    <Col span={4}>
                      <Form.Item label="代理池数量" name="ipProxyPoolCount">
                        <InputNumber min={1} max={200} style={{ width: '100%' }} addonAfter="个" />
                      </Form.Item>
                    </Col>
                  </>)}
                </Row>
                {enableIPProxy && ipProxyProvider === 'kuaidaili' && (
                  <Row gutter={16}>
                    <Col span={6}>
                      <Form.Item label="KDL Secret ID" name="proxyKdlSecretId">
                        <Input.Password placeholder="已设置则留空不变" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item label="KDL Signature" name="proxyKdlSignature">
                        <Input.Password placeholder="已设置则留空不变" />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item label="KDL 用户名" name="proxyKdlUsername">
                        <Input />
                      </Form.Item>
                    </Col>
                    <Col span={6}>
                      <Form.Item label="KDL 密码" name="proxyKdlPassword">
                        <Input.Password placeholder="已设置则留空不变" />
                      </Form.Item>
                    </Col>
                  </Row>
                )}
                {enableIPProxy && ipProxyProvider === 'wandou' && (
                  <Row gutter={16}>
                    <Col span={8}>
                      <Form.Item label="豌豆 App Key" name="proxyWandouAppKey">
                        <Input.Password placeholder="已设置则留空不变" />
                      </Form.Item>
                    </Col>
                  </Row>
                )}

                {/* 平台排序策略 */}
                <Divider orientation="left" plain style={{ color: '#888', fontSize: 12 }}>平台排序策略</Divider>
                <Row gutter={16}>
                  <Col span={6}>
                    <Form.Item label="小红书排序" name="xhsSortType">
                      <Select options={XHS_SORT_OPTIONS} />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="微博搜索类型" name="weiboSearchType">
                      <Select options={WB_SEARCH_OPTIONS} />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="抖音排序" name="dySortType">
                      <Select options={DY_SORT_OPTIONS} />
                    </Form.Item>
                  </Col>
                </Row>
                <Row gutter={16}>
                  <Col span={6}>
                    <Form.Item label="知乎排序" name="zhihuSort">
                      <Select options={ZHIHU_SORT_OPTIONS} />
                    </Form.Item>
                  </Col>
                  <Col span={6}>
                    <Form.Item label="知乎时间范围" name="zhihuSearchTime">
                      <Select options={ZHIHU_TIME_OPTIONS} />
                    </Form.Item>
                  </Col>
                </Row>

                <div style={{ paddingTop: 8 }}>
                  <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={crawlerSaving}>
                    保存参数配置
                  </Button>
                </div>
              </Form>
            </Spin>
          </Card>
        </Col>

        {/* 平台 Cookie 管理 */}
        <Col span={24}>
          <Card className={ui.panelCard} title={<><KeyOutlined style={{ marginRight: 6 }} />平台 Cookie 管理</>}
            extra={<Text type="secondary" style={{ fontSize: 12 }}>Cookie 用于已登录身份访问，定期更新以防失效</Text>}
          >
            <Alert className={ui.infoBanner} message="Cookie 安全说明"
              description="Cookie 以加密方式存储于数据库，展示时已脱敏。更新后立即对下次爬虫任务生效，无需重启服务。"
              type="info" showIcon style={{ marginBottom: 16 }} />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {PLATFORMS.map(({ code, name, color }) => {
                const info = cookies[code]
                const isSet = info?.set ?? false
                return (
                  <div key={code} style={{
                    display: 'flex', alignItems: 'center', gap: 12,
                    padding: '10px 14px', borderRadius: 8,
                    background: 'rgba(0,0,0,0.02)', border: '1px solid #f0f0f0',
                  }}>
                    <div style={{
                      width: 8, height: 8, borderRadius: '50%',
                      background: color, flexShrink: 0,
                    }} />
                    <Text strong style={{ width: 72, flexShrink: 0 }}>{name}</Text>
                    {isSet ? (
                      <>
                        <Tag className={ui.softTagSage} style={{ fontFamily: 'monospace', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {info.masked}
                        </Tag>
                        <Tag className={ui.softTagBlue} style={{ flexShrink: 0 }}>已设置</Tag>
                      </>
                    ) : (
                      <Tag className={ui.softTagNeutral} style={{ flexShrink: 0 }}>未设置</Tag>
                    )}
                    <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
                      <Button size="small" type="primary" ghost
                        onClick={() => handleOpenCookieModal(code, name)}>
                        {isSet ? '更新' : '设置'} Cookie
                      </Button>
                      {isSet && (
                        <Button size="small" danger ghost
                          onClick={() => void handleClearCookie(code, name)}>
                          清除
                        </Button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </Card>
        </Col>
      </Row>

      {/* Cookie 编辑弹窗 */}
      <Modal
        title={`更新 ${cookieModal.name} Cookie`}
        open={cookieModal.open}
        onCancel={() => setCookieModal({ open: false, platform: '', name: '' })}
        onOk={() => void handleSaveCookie()}
        okText="保存"
        cancelText="取消"
        confirmLoading={cookieSaving}
        width={600}
      >
        <div style={{ marginBottom: 12 }}>
          <Text type="secondary" style={{ fontSize: 13 }}>
            请粘贴完整的 Cookie 字符串（通常从浏览器开发者工具 Network 面板复制）
          </Text>
        </div>
        <Input.TextArea
          value={cookieInput}
          onChange={(e) => setCookieInput(e.target.value)}
          placeholder="粘贴 Cookie 字符串..."
          rows={5}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
        />
      </Modal>
    </div>
  )
}

export default CrawlerConfigPage