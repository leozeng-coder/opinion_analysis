/** 全站 ECharts 配色 — 与 tokens.css 一致 */
export const CHART = {
  positive: '#42C48C',
  positiveArea: 'rgba(66, 196, 140, 0.22)',
  neutral: '#4D93E8',
  neutralArea: 'rgba(77, 147, 232, 0.22)',
  negative: '#EC6B6B',
  negativeArea: 'rgba(236, 107, 107, 0.22)',
  bar: '#6EB0F5',
  pie: ['#6EB0F5', '#42C48C', '#E8A84A', '#8B5CF6', '#22D3EE', '#EC6B6B'],
}

/** 词云专用色板 — 色相跨度大、彼此易区分 */
export const WORD_CLOUD_PALETTE = [
  '#4D93E8', // 蓝
  '#42C48C', // 绿
  '#E8A84A', // 琥珀
  '#EC6B6B', // 珊瑚红
  '#8B5CF6', // 紫
  '#F472B6', // 粉
  '#22D3EE', // 青
  '#FB923C', // 橙
  '#6366F1', // 靛
  '#14B8A6', //  teal
  '#E879F9', // 品红
  '#84CC16', //  lime
  '#0EA5E9', // 天蓝
  '#F43F5E', // 玫红
  '#A855F7', // 浅紫
  '#10B981', // 祖母绿
] as const

/** 按标签名哈希取色，同一词刷新后颜色不变 */
export function wordCloudColor(tag: string): string {
  let hash = 0
  for (let i = 0; i < tag.length; i++) {
    hash = (hash * 31 + tag.charCodeAt(i)) >>> 0
  }
  return WORD_CLOUD_PALETTE[hash % WORD_CLOUD_PALETTE.length]
}

export const chartTooltip = {  backgroundColor: 'rgba(255,255,255,0.96)',
  borderColor: 'rgba(15,23,42,0.08)',
  textStyle: { color: 'rgba(15,23,42,0.75)', fontSize: 12 },
}

export const chartAxis = {
  line: { lineStyle: { color: 'rgba(15,23,42,0.08)' } },
  label: { color: 'rgba(15,23,42,0.45)', fontSize: 11 },
  splitLine: { lineStyle: { color: 'rgba(15,23,42,0.05)', type: 'dashed' as const } },
}
