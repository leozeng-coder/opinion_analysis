package service

import "strings"

// SpiderKeyToSyncCode 将工作流/爬虫 spider 标识映射为同步器平台代码
func SpiderKeyToSyncCode(spider string) string {
	mapping := map[string]string{
		"broad-topic":    "zhihu",
		"deep-sentiment": "zhihu",
		"zhihu":          "zhihu",
		"xiaohongshu":    "xhs",
		"xhs":            "xhs",
		"douyin":         "dy",
		"dy":             "dy",
		"kuaishou":       "ks",
		"ks":             "ks",
		"bilibili":       "bili",
		"bili":           "bili",
		"weibo":          "wb",
		"wb":             "wb",
		"tieba":          "tieba",
	}
	key := strings.ToLower(strings.TrimSpace(spider))
	if code, ok := mapping[key]; ok {
		return code
	}
	return key
}

// SyncCodeToArticlePlatform 同步器平台代码 → articles 表 platform 字段值
func SyncCodeToArticlePlatform(code string) string {
	mapping := map[string]string{
		"xhs":    "xhs",
		"dy":     "douyin",
		"ks":     "kuaishou",
		"bili":   "bilibili",
		"wb":     "weibo",
		"tieba":  "tieba",
		"zhihu":  "zhihu",
	}
	if p, ok := mapping[code]; ok {
		return p
	}
	return code
}

// ResolveSyncCodes 去重解析 spider/平台标识列表为同步器代码
func ResolveSyncCodes(keys []string) []string {
	seen := make(map[string]struct{})
	var codes []string
	for _, key := range keys {
		code := SpiderKeyToSyncCode(key)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

// ResolveArticlePlatforms 同步器代码 → articles.platform 查询值
func ResolveArticlePlatforms(syncCodes []string) []string {
	seen := make(map[string]struct{})
	var platforms []string
	for _, code := range syncCodes {
		p := SyncCodeToArticlePlatform(code)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		platforms = append(platforms, p)
	}
	return platforms
}
