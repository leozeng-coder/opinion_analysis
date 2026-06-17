/**
 * 后台管理端平台标识的唯一来源，展示名与用户端保持一致。
 *
 * 一个平台有两种标识：
 *   - 短码 code：xhs dy ks bili wb tieba zhihu（爬虫/配置态、/api/platform/list 返回）
 *   - 入库值 article：xhs douyin kuaishou bilibili weibo tieba zhihu（articles.platform）
 * 两者在 xhs/tieba/zhihu 上相同，其余四个不同。
 *
 * LABEL 同时以入库值和短码为 key，任意标识都能解析为中文名。
 */

interface PlatformDef {
  code: string
  article: string
  label: string
  /** 品牌色，用于爬虫配置等需要平台主题色的场景 */
  brandColor: string
}

/** 七大平台权威定义，顺序即默认展示顺序 */
export const PLATFORMS: PlatformDef[] = [
  { code: 'xhs',   article: 'xhs',      label: '小红书', brandColor: '#ff2442' },
  { code: 'dy',    article: 'douyin',   label: '抖音',   brandColor: '#161823' },
  { code: 'ks',    article: 'kuaishou', label: '快手',   brandColor: '#ff5500' },
  { code: 'bili',  article: 'bilibili', label: 'B站',    brandColor: '#00a1d6' },
  { code: 'wb',    article: 'weibo',    label: '微博',   brandColor: '#e6162d' },
  { code: 'tieba', article: 'tieba',    label: '贴吧',   brandColor: '#2468f2' },
  { code: 'zhihu', article: 'zhihu',    label: '知乎',   brandColor: '#0066ff' },
]

/** 以入库值和短码双重索引的中文名表 */
export const PLATFORM_LABEL: Record<string, string> = {}
for (const p of PLATFORMS) {
  PLATFORM_LABEL[p.article] = p.label
  PLATFORM_LABEL[p.code] = p.label
}

/** 取平台中文展示名；接受入库值或短码，未知则原样返回 */
export const platformLabel = (p: string) => PLATFORM_LABEL[p] ?? p
