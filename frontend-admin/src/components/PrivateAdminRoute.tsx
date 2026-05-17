import React from 'react'
import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'

const PrivateAdminRoute: React.FC = () => {
  const { token, user } = useAuthStore()
  if (!token || !user) {
    return <Navigate to="/login" replace />
  }
  if (user.role !== 'admin') {
    return <Navigate to="/login" replace state={{ error: '需要 admin 角色才能访问管理后台' }} />
  }
  return <Outlet />
}

export default PrivateAdminRoute
