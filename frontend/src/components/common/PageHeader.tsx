import React from 'react'
import { Typography } from 'antd'
import page from '@/styles/page.module.css'

const { Title, Text } = Typography

export interface PageHeaderProps {
  title: string
  subtitle?: string
  icon?: React.ReactNode
  extra?: React.ReactNode
}

const PageHeader: React.FC<PageHeaderProps> = ({ title, subtitle, icon, extra }) => (
  <div className={page.pageHeader}>
    <div className={page.pageHeaderMain}>
      {icon && <div className={page.pageHeaderIcon}>{icon}</div>}
      <div>
        <Title level={4} className={page.pageTitle}>{title}</Title>
        {subtitle && <Text className={page.pageSubtitle}>{subtitle}</Text>}
      </div>
    </div>
    {extra && <div className={page.pageExtra}>{extra}</div>}
  </div>
)

export default PageHeader
