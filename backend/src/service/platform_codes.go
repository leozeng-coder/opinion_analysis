package service

import "opinion-analysis/pkg/platform"

// 本文件是 pkg/platform 的薄转发层，保留历史函数签名以兼容现有调用方。
// 平台标识的权威定义与换算逻辑全部在 pkg/platform，新代码请直接引用该包。

// SpiderKeyToSyncCode 将工作流/爬虫 spider 标识映射为同步器平台代码
func SpiderKeyToSyncCode(spider string) string {
	return platform.CodeOf(spider)
}

// SyncCodeToArticlePlatform 同步器平台代码 → articles 表 platform 字段值
func SyncCodeToArticlePlatform(code string) string {
	return platform.ArticleValue(code)
}

// ResolveSyncCodes 去重解析 spider/平台标识列表为同步器代码
func ResolveSyncCodes(keys []string) []string {
	return platform.ResolveCodes(keys)
}

// ResolveArticlePlatforms 同步器代码 → articles.platform 查询值
func ResolveArticlePlatforms(syncCodes []string) []string {
	return platform.ResolveArticleValues(syncCodes)
}

// SyncCodeToSourceTable 同步器平台代码 → MediaCrawler 源表名
func SyncCodeToSourceTable(code string) string {
	return platform.SourceTable(code)
}

// SyncCodeToCommentTable 同步器平台代码 → MediaCrawler 评论源表名
func SyncCodeToCommentTable(code string) string {
	return platform.CommentTable(code)
}
