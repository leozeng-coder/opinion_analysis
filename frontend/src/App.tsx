import React, { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Spin } from 'antd'
import AppLayout from '@/components/layout/AppLayout'
import PrivateRoute from '@/components/PrivateRoute'

const LoginPage = lazy(() => import('@/pages/login/LoginPage'))
const RegisterPage = lazy(() => import('@/pages/login/RegisterPage'))
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'))
const OpinionPage = lazy(() => import('@/pages/opinion/OpinionPage'))
const TopicsPage = lazy(() => import('@/pages/topics/TopicsPage'))
const AlertsPage = lazy(() => import('@/pages/alerts/AlertsPage'))
const StatsPage = lazy(() => import('@/pages/stats/StatsPage'))
const CrawlerPage = lazy(() => import('@/pages/crawler/CrawlerPage'))
const AiAssistantPage = lazy(() => import('@/pages/assistant/AiAssistantPage'))
const PlatformDataPage = lazy(() => import('@/pages/platform/PlatformDataPage'))
const WorkflowListPage = lazy(() => import('@/pages/workflow/WorkflowListPage'))
const WorkflowEditorPage = lazy(() => import('@/pages/workflow/WorkflowEditorPage'))
const WorkflowExecutionPage = lazy(() => import('@/pages/workflow/WorkflowExecutionPage'))

const fallback = <Spin size="large" style={{ display: 'block', marginTop: 100, textAlign: 'center' }} />

const App: React.FC = () => (
  <BrowserRouter>
    <Suspense fallback={fallback}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route element={<PrivateRoute />}>
          <Route element={<AppLayout />}>
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/opinion" element={<OpinionPage />} />
            <Route path="/topics" element={<TopicsPage />} />
            <Route path="/alerts" element={<AlertsPage />} />
            <Route path="/stats" element={<StatsPage />} />
            <Route path="/crawler" element={<CrawlerPage />} />
            <Route path="/platform" element={<PlatformDataPage />} />
            <Route path="/assistant" element={<AiAssistantPage />} />
            <Route path="/workflows" element={<WorkflowListPage />} />
            <Route path="/workflows/:id/edit" element={<WorkflowEditorPage />} />
            <Route path="/workflows/new" element={<WorkflowEditorPage />} />
            <Route path="/workflows/:id/executions" element={<WorkflowExecutionPage />} />
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  </BrowserRouter>
)

export default App
