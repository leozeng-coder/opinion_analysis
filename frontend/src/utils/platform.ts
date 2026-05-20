export const PLATFORM_LABEL: Record<string, string> = {
  weibo: '微博热搜',
  zhihu: '知乎热榜',
  'bilibili-hot-search': 'B站热搜',
  toutiao: '今日头条',
  douyin: '抖音热榜',
  coolapk: '酷安热榜',
  tieba: '百度贴吧',
  wallstreetcn: '华尔街见闻',
  thepaper: '澎湃新闻',
  'cls-hot': '财联社',
  xueqiu: '雪球热榜',
  kuaishou: '快手热榜',
  xhs: '小红书',
  dy: '抖音',
  ks: '快手',
  bili: 'B站',
  wb: '微博',
  weixin: '微信',
  news: '新闻',
  forum: '论坛',
}

/**
 * 各平台柔和配色 — 低饱和、高明度，色相刻意错开以便区分
 * （微博偏珊瑚、小红书偏玫粉、抖音偏紫粉、快手偏杏橙…）
 */
export const PLATFORM_COLOR: Record<string, string> = {
  // MediaCrawler 短码
  xhs: '#EAB4C4',       // 玫粉
  dy: '#E8C0D8',        // 紫粉
  ks: '#F2D4A8',        // 杏橙
  bili: '#F0C0D0',      // 樱花粉
  wb: '#E8B0B0',        // 珊瑚
  tieba: '#A8BCE8',     // 雾蓝紫
  zhihu: '#90C4F0',     // 天蓝

  // 新闻聚合源
  weibo: '#E8B0B0',
  douyin: '#E8C0D8',
  kuaishou: '#F2D4A8',
  'bilibili-hot-search': '#98D4E8', // 青蓝（与 bili 樱花粉区分）
  toutiao: '#F0C8A0',   // 蜜橙
  coolapk: '#98D8B8',   // 薄荷绿
  wallstreetcn: '#B0B8C8', // 石板灰蓝
  thepaper: '#A0B0C8',  // 钢蓝
  'cls-hot': '#E0A8B0', // 干玫瑰
  xueqiu: '#A8C4E8',    // 淡蓝

  // 通用分类
  weixin: '#A0DCC0',    // 微信绿
  news: '#B8D4F8',      // 淡蓝
  forum: '#C4B0E8',     // 薰衣草
}

/** 未知平台兜底 — 柔和且色相分散 */
const SOFT_PLATFORM_FALLBACK = [
  '#A8C8F0', // 蓝
  '#98D8B8', // 绿
  '#F0C898', // 黄橙
  '#E8B0C8', // 粉
  '#C0A8E8', // 紫
  '#98D0E8', // 青
  '#E8C0A8', // 杏
  '#A8D8C8', // 蓝绿
  '#F0B8B8', // 珊瑚
  '#B8C8E8', // 雾蓝
] as const

function platformSoftFallback(platform: string): string {
  let hash = 0
  for (let i = 0; i < platform.length; i++) {
    hash = (hash * 31 + platform.charCodeAt(i)) >>> 0
  }
  return SOFT_PLATFORM_FALLBACK[hash % SOFT_PLATFORM_FALLBACK.length]
}

export const platformLabel = (p: string) => PLATFORM_LABEL[p] ?? p

/** 取平台色；未知平台用柔和兜底色（稳定、互异） */
export const platformColor = (p: string) => PLATFORM_COLOR[p] ?? platformSoftFallback(p)
