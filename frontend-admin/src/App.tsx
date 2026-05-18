import React, { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Spin } from 'antd'
import AdminLayout from '@/components/AdminLayout'
import PrivateAdminRoute from '@/components/PrivateAdminRoute'

const LoginPage = lazy(() => import('@/pages/login/LoginPage'))
const UsersPage = lazy(() => import('@/pages/users/UsersPage'))
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'))
const SystemPage = lazy(() => import('@/pages/system/SystemPage'))
const CrawlerPage = lazy(() => import('@/pages/crawler/CrawlerPage'))
const TaggerPage = lazy(() => import('@/pages/tagger/TaggerPage'))
const DataSourcePage = lazy(() => import('@/pages/datasource/DataSourcePage'))
const AuditPage = lazy(() => import('@/pages/audit/AuditPage'))
const RagKBPage = lazy(() => import('@/pages/rag/RagKBPage'))

const fallback = <Spin size="large" style={{ display: 'block', marginTop: 100, textAlign: 'center' }} />

// isProd 时 BrowserRouter basename 对应 /admin/，dev 用 /
const basename = typeof import.meta !== 'undefined' && import.meta.env?.PROD ? '/admin' : '/'

const App: React.FC = () => (
  <BrowserRouter basename={basename}>
    <Suspense fallback={fallback}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<PrivateAdminRoute />}>
          <Route element={<AdminLayout />}>
            <Route path="/users" element={<UsersPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/system" element={<SystemPage />} />
            <Route path="/crawler" element={<CrawlerPage />} />
            <Route path="/tagger" element={<TaggerPage />} />
            <Route path="/datasources" element={<DataSourcePage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/rag-kb" element={<RagKBPage />} />
            <Route path="/" element={<Navigate to="/system" replace />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  </BrowserRouter>
)

export default App

