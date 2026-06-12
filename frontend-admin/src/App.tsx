import React, { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Spin } from 'antd'
import AdminLayout from '@/components/AdminLayout'
import PrivateAdminRoute from '@/components/PrivateAdminRoute'

const LoginPage = lazy(() => import('@/pages/login/LoginPage'))
const UsersPage = lazy(() => import('@/pages/users/UsersPage'))
const SystemPage = lazy(() => import('@/pages/system/SystemPage'))
const SystemConfigPage = lazy(() => import('@/pages/config/SystemConfigPage'))
const NotifyPage = lazy(() => import('@/pages/config/NotifyPage'))
const PlatformSyncPage = lazy(() => import('@/pages/platform/PlatformSyncPage'))
const TaggerPage = lazy(() => import('@/pages/ai/TaggerPage'))
const RagPage = lazy(() => import('@/pages/ai/RagPage'))
const AuditPage = lazy(() => import('@/pages/audit/AuditPage'))
const CrawlerConfigPage = lazy(() => import('@/pages/config/CrawlerConfigPage'))

const fallback = <Spin size="large" style={{ display: 'block', marginTop: 100, textAlign: 'center' }} />

const basename = typeof import.meta !== 'undefined' && import.meta.env?.PROD ? '/admin' : '/'

const App: React.FC = () => (
  <BrowserRouter basename={basename}>
    <Suspense fallback={fallback}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<PrivateAdminRoute />}>
          <Route element={<AdminLayout />}>
            <Route path="/system" element={<SystemPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/data/platform-sync" element={<PlatformSyncPage />} />
            {/* AI 引擎 */}
            <Route path="/ai/tagger" element={<TaggerPage />} />
            <Route path="/ai/rag" element={<RagPage />} />
            {/* 系统设置 */}
            <Route path="/config/system" element={<SystemConfigPage />} />
            <Route path="/config/notify" element={<NotifyPage />} />
            <Route path="/config/crawler" element={<CrawlerConfigPage />} />
            {/* 审计日志 */}
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/config" element={<Navigate to="/config/system" replace />} />
            <Route path="/settings" element={<Navigate to="/config/system" replace />} />
            <Route path="/tagger" element={<Navigate to="/ai/tagger" replace />} />
            <Route path="/crawler" element={<Navigate to="/data/platform-sync" replace />} />
            <Route path="/" element={<Navigate to="/system" replace />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  </BrowserRouter>
)

export default App
