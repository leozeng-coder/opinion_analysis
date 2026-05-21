import React, { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Spin } from 'antd'
import AdminLayout from '@/components/AdminLayout'
import PrivateAdminRoute from '@/components/PrivateAdminRoute'

const LoginPage = lazy(() => import('@/pages/login/LoginPage'))
const UsersPage = lazy(() => import('@/pages/users/UsersPage'))
const SystemPage = lazy(() => import('@/pages/system/SystemPage'))
const SystemConfigPage = lazy(() => import('@/pages/config/SystemConfigPage'))
const AIConfigPage = lazy(() => import('@/pages/config/AIConfigPage'))
const CrawlerConfigPage = lazy(() => import('@/pages/config/CrawlerConfigPage'))
const TasksPage = lazy(() => import('@/pages/tasks/TasksPage'))
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
            <Route path="/system" element={<SystemPage />} />
            <Route path="/config/system" element={<SystemConfigPage />} />
            <Route path="/config/ai" element={<AIConfigPage />} />
            <Route path="/config/crawler" element={<CrawlerConfigPage />} />
            <Route path="/tasks" element={<TasksPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/rag-kb" element={<RagKBPage />} />
            <Route path="/datasources" element={<DataSourcePage />} />
            <Route path="/audit" element={<AuditPage />} />
            {/* 旧路由重定向 */}
            <Route path="/config" element={<Navigate to="/config/system" replace />} />
            <Route path="/settings" element={<Navigate to="/config/system" replace />} />
            <Route path="/tagger" element={<Navigate to="/tasks" replace />} />
            <Route path="/crawler" element={<Navigate to="/config/crawler" replace />} />
            <Route path="/" element={<Navigate to="/system" replace />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  </BrowserRouter>
)

export default App
