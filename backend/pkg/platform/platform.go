// Package platform 是全系统平台标识的唯一权威来源。
//
// 系统内一个平台存在三种标识，本包负责它们之间的换算与展示名：
//   - code        同步器/爬虫/配置使用的短码：xhs dy ks bili wb tieba zhihu
//   - article     articles.platform 列实际存储值：xhs douyin kuaishou bilibili weibo tieba zhihu
//   - 中文展示名   面向用户统一展示：小红书 抖音 快手 B站 微博 贴吧 知乎
//
// 注意：xhs/tieba/zhihu 三个平台 code 与 article 值相同；其余四个不同。
// 本包不依赖任何项目内其他包，可被 service / digest 等任意层引用，避免循环依赖。
package platform

import "strings"

// Info 描述一个平台的完整标识三元组。
type Info struct {
	Code        string // 短码，如 "wb"
	Article     string // articles.platform 存储值，如 "weibo"
	DisplayName string // 中文展示名，如 "微博"
	SourceTable string // MediaCrawler 主源表，如 "weibo_note"
	CommentTable string // MediaCrawler 评论源表，如 "weibo_note_comment"
}

// All 是七大平台的权威定义，顺序即默认展示顺序。
var All = []Info{
	{Code: "xhs", Article: "xhs", DisplayName: "小红书", SourceTable: "xhs_note", CommentTable: "xhs_note_comment"},
	{Code: "dy", Article: "douyin", DisplayName: "抖音", SourceTable: "douyin_aweme", CommentTable: "douyin_aweme_comment"},
	{Code: "ks", Article: "kuaishou", DisplayName: "快手", SourceTable: "kuaishou_video", CommentTable: "kuaishou_video_comment"},
	{Code: "bili", Article: "bilibili", DisplayName: "B站", SourceTable: "bilibili_video", CommentTable: "bilibili_video_comment"},
	{Code: "wb", Article: "weibo", DisplayName: "微博", SourceTable: "weibo_note", CommentTable: "weibo_note_comment"},
	{Code: "tieba", Article: "tieba", DisplayName: "贴吧", SourceTable: "tieba_note", CommentTable: "tieba_comment"},
	{Code: "zhihu", Article: "zhihu", DisplayName: "知乎", SourceTable: "zhihu_content", CommentTable: "zhihu_comment"},
}

var (
	byCode    = map[string]Info{}
	byArticle = map[string]Info{}
	// aliasToCode 把各种 spider/平台别名归一到短码。
	aliasToCode = map[string]string{
		"broad-topic":    "zhihu",
		"deep-sentiment": "zhihu",
		"xiaohongshu":    "xhs",
		"douyin":         "dy",
		"kuaishou":       "ks",
		"bilibili":       "bili",
		"weibo":          "wb",
	}
)

func init() {
	for _, p := range All {
		byCode[p.Code] = p
		byArticle[p.Article] = p
		// code 与 article 本身也是合法别名，归一到 code
		aliasToCode[p.Code] = p.Code
		aliasToCode[p.Article] = p.Code
	}
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// CodeOf 把任意平台别名（短码 / 入库值 / spider 名）归一为短码。
// 无法识别时原样返回归一化后的输入。
func CodeOf(alias string) string {
	key := norm(alias)
	if code, ok := aliasToCode[key]; ok {
		return code
	}
	return key
}

// ArticleValue 把任意平台别名换算为 articles.platform 存储值。
// 无法识别时原样返回归一化后的输入。
func ArticleValue(alias string) string {
	if p, ok := byCode[CodeOf(alias)]; ok {
		return p.Article
	}
	return norm(alias)
}

// DisplayName 返回平台中文展示名；接受短码或入库值。
// 无法识别时原样返回输入。
func DisplayName(alias string) string {
	if p, ok := byCode[CodeOf(alias)]; ok {
		return p.DisplayName
	}
	return alias
}

// SourceTable 返回主源表名；接受短码或入库值。无法识别返回空串。
func SourceTable(alias string) string {
	if p, ok := byCode[CodeOf(alias)]; ok {
		return p.SourceTable
	}
	return ""
}

// CommentTable 返回评论源表名；接受短码或入库值。无法识别返回空串。
func CommentTable(alias string) string {
	if p, ok := byCode[CodeOf(alias)]; ok {
		return p.CommentTable
	}
	return ""
}

// ResolveCodes 去重解析平台别名列表为短码列表。
func ResolveCodes(aliases []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, a := range aliases {
		code := CodeOf(a)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

// ResolveArticleValues 去重解析平台别名列表为 articles.platform 查询值列表。
func ResolveArticleValues(aliases []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, a := range aliases {
		v := ArticleValue(a)
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
