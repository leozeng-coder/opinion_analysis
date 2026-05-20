import type { ThemeConfig } from 'antd'

/** 全站 Ant Design 主题 — 与 tokens.css 一致 */
export const appTheme: ThemeConfig = {
  token: {
    colorPrimary: '#4D93E8',
    colorSuccess: '#42C48C',
    colorWarning: '#E8A84A',
    colorError: '#EC6B6B',
    colorInfo: '#4D93E8',
    borderRadius: 10,
    colorBgContainer: '#ffffff',
    colorBgLayout: '#f2f6fc',
    colorBorder: 'rgba(15, 23, 42, 0.07)',
    colorText: 'rgba(15, 23, 42, 0.84)',
    colorTextSecondary: 'rgba(15, 23, 42, 0.50)',
    colorTextTertiary: 'rgba(15, 23, 42, 0.38)',
    boxShadow: '0 1px 2px rgba(15, 23, 42, 0.04), 0 4px 16px rgba(15, 23, 42, 0.05)',
    boxShadowSecondary: '0 4px 20px rgba(77, 147, 232, 0.12)',
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
  },
  components: {
    Layout: {
      bodyBg: '#f2f6fc',
      headerBg: '#ffffff',
      siderBg: '#1e293b',
      triggerBg: '#172033',
    },
    Menu: {
      darkItemBg: '#1e293b',
      darkSubMenuItemBg: '#172033',
      darkItemSelectedBg: 'rgba(77, 147, 232, 0.24)',
    },
    Card: {
      borderRadiusLG: 12,
    },
    Table: {
      borderRadius: 12,
      headerBg: '#f8fafc',
      rowHoverBg: '#f8fafc',
    },
    Tabs: {
      inkBarColor: '#4D93E8',
      itemSelectedColor: '#2D7AD8',
    },
    Button: {
      primaryShadow: '0 2px 0 rgba(77, 147, 232, 0.15)',
    },
    Tag: {
      borderRadiusSM: 6,
    },
  },
}
