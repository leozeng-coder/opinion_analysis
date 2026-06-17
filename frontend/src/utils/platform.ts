/**
 * 全前端平台标识的唯一来源。
 *
 * 系统内一个平台有两种标识：
 *   - 短码 code：xhs dy ks bili wb tieba zhihu（爬虫/配置态使用）
 *   - 入库值 article：xhs douyin kuaishou bilibili weibo tieba zhihu
 *     （articles.platform 实际存储，文章/图表/列表数据携带的就是此值）
 * 两者在 xhs/tieba/zhihu 上相同，其余四个不同。
 *
 * 下面的 LABEL/COLOR 同时以「入库值」和「短码」为 key，
 * 因此无论拿到哪种标识，platformLabel/platformColor 都能正确解析。
 */

interface PlatformDef {
  code: string
  article: string
  label: string
  color: string
}

/** 七大平台权威定义，顺序即默认展示顺序 */
export const PLATFORMS: PlatformDef[] = [
  { code: 'xhs',   article: 'xhs',      label: '小红书', color: '#EAB4C4' }, // 玫粉
  { code: 'dy',    article: 'douyin',   label: '抖音',   color: '#E8C0D8' }, // 紫粉
  { code: 'ks',    article: 'kuaishou', label: '快手',   color: '#F2D4A8' }, // 杏橙
  { code: 'bili',  article: 'bilibili', label: 'B站',    color: '#F0C0D0' }, // 樱花粉
  { code: 'wb',    article: 'weibo',    label: '微博',   color: '#E8B0B0' }, // 珊瑚
  { code: 'tieba', article: 'tieba',    label: '贴吧',   color: '#A8BCE8' }, // 雾蓝紫
  { code: 'zhihu', article: 'zhihu',    label: '知乎',   color: '#90C4F0' }, // 天蓝
]

// 同时以入库值和短码为 key 建索引，任意标识都能命中
export const PLATFORM_LABEL: Record<string, string> = {}
export const PLATFORM_COLOR: Record<string, string> = {}
for (const p of PLATFORMS) {
  PLATFORM_LABEL[p.article] = p.label
  PLATFORM_LABEL[p.code] = p.label
  PLATFORM_COLOR[p.article] = p.color
  PLATFORM_COLOR[p.code] = p.color
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

/** 取平台中文展示名；接受入库值或短码，未知则原样返回 */
export const platformLabel = (p: string) => PLATFORM_LABEL[p] ?? p

/** 取平台色；未知平台用柔和兜底色（稳定、互异） */
export const platformColor = (p: string) => PLATFORM_COLOR[p] ?? platformSoftFallback(p)
