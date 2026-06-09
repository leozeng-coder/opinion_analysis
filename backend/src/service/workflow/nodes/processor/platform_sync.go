package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	platformSync "opinion-analysis/src/service"
	"opinion-analysis/src/service/workflow/nodes"
)

// PlatformSyncNode 将 MediaCrawler 平台表同步到 articles 中心表。
//
// 关键设计：基于「持久化偏移量（platform_sync_offset）」做主键增量，
// 偏移量记录每个平台已同步进 articles 的源表最大 id，只处理 id > offset 的新行，
// 成功后推进偏移量。相比旧的「爬虫临时 baseline」方案，不依赖 crawler_run 与
// platform_sync 的严格配对，任何新增行迟早会被下一次同步捕获，从根本上避免漏行；
// 同时复杂度为 O(新增行数)，数据量增长也不会变慢。
type PlatformSyncNode struct {
	*nodes.BaseNode
	db *gorm.DB
}

func NewPlatformSyncNode(db *gorm.DB) *PlatformSyncNode {
	return &PlatformSyncNode{
		BaseNode: nodes.NewBaseNode("platform_sync"),
		db:       db,
	}
}

func (n *PlatformSyncNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *PlatformSyncNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	syncMode := n.GetString(config, "syncMode", "incremental")
	enableSentiment := n.GetBool(config, "enableSentiment", false)
	syncSinceMinutes := n.GetInt(config, "syncSinceMinutes", 0)

	// 解析平台：节点配置 > 上游 platforms > 上游 syncPlatformCodes > 默认 zhihu
	platformKeys := n.GetStringSlice(config, "platforms")
	if len(platformKeys) == 0 {
		platformKeys = nodes.GetStringSliceFromInput(input, "platforms")
	}
	if len(platformKeys) == 0 {
		platformKeys = nodes.GetStringSliceFromInput(input, "syncPlatformCodes")
	}
	syncCodes := platformSync.ResolveSyncCodes(platformKeys)
	if len(syncCodes) == 0 {
		syncCodes = platformSync.ResolveSyncCodes([]string{"zhihu"})
	}

	// 从上游获取过滤后的源表 ID 列表（数据过滤节点传递）
	includeSourceIDs := n.extractIncludeSourceIDs(input)

	// 从上游获取爬虫启动前的源表 baseline（爬虫节点传递），用于只同步本次爬取新增行
	sourceBaselines := n.extractSourceBaselines(input)

	// 从上游获取 topics 列表（爬虫节点传递），取第一个作为 topic
	var topic string
	if topics := nodes.GetStringSliceFromInput(input, "topics"); len(topics) > 0 {
		topic = topics[0]
	}

	log.Printf("[PlatformSyncNode] mode=%s syncCodes=%v topic=%s includeSourceIDs=%d",
		syncMode, syncCodes, topic, len(includeSourceIDs))

	syncSvc := platformSync.NewPlatformSyncService(n.db)
	results := make(map[string]interface{})
	totalNew := 0
	hasErrors := false
	var allInsertedIDs []int64

	for _, code := range syncCodes {
		var result *platformSync.SyncResult
		var syncErr error
		var strategy string

		switch {
		case len(includeSourceIDs) > 0:
			strategy = "filtered"
			result, syncErr = syncSvc.SyncPlatformBySourceIDsWithTopic(ctx, code, includeSourceIDs, topic, enableSentiment)
		case syncMode == "full":
			strategy = "full"
			result, syncErr = syncSvc.SyncPlatformFullWithTopic(ctx, code, topic, enableSentiment)
		case syncSinceMinutes > 0:
			since := time.Now().Add(-time.Duration(syncSinceMinutes) * time.Minute)
			strategy = fmt.Sprintf("since=%dm", syncSinceMinutes)
			result, syncErr = syncSvc.SyncPlatformSinceWithTopic(ctx, code, since, topic, enableSentiment)
		default:
			if baseline, ok := sourceBaselines[code]; ok {
				strategy = fmt.Sprintf("baseline=%d", baseline)
				result, syncErr = syncSvc.SyncPlatformFromBaselineWithTopic(ctx, code, baseline, topic, enableSentiment)
			} else {
				strategy = "offset"
				result, syncErr = syncSvc.SyncPlatformByOffsetWithTopic(ctx, code, topic, enableSentiment)
			}
		}
		if syncErr != nil {
			return nil, n.WrapError(fmt.Sprintf("sync platform %s failed", code), syncErr)
		}

		totalNew += result.NewCount
		if result.ErrorCount > 0 {
			hasErrors = true
		}
		allInsertedIDs = append(allInsertedIDs, result.InsertedIDs...)
		results[code] = map[string]interface{}{
			"strategy":     strategy,
			"newCount":     result.NewCount,
			"skippedCount": result.SkippedCount,
			"errorCount":   result.ErrorCount,
			"status":       result.Status,
		}
		log.Printf("[PlatformSyncNode] %s strategy=%s new=%d skipped=%d errors=%d",
			code, strategy, result.NewCount, result.SkippedCount, result.ErrorCount)
	}

	status := "synced"
	if hasErrors {
		status = "partial_success"
	}

	produced := map[string]interface{}{
		"articleIds":        nodes.PackArticleIDs(allInsertedIDs),
		"articlesCount":    len(allInsertedIDs),
		"syncMode":         syncMode,
		"syncPlatforms":    syncCodes,
		"syncPlatformCodes": syncCodes,
		"syncResults":      results,
		"syncNewCount":     totalNew,
		"status":           status,
	}

	output := nodes.CarryForward(input, produced)

	log.Printf("[PlatformSyncNode] done: newRecords=%d articleIds=%d",
		totalNew, len(allInsertedIDs))

	return output, nil
}

// extractSourceBaselines 从上游 crawler_run 节点的输出中提取各平台的源表 baseline（爬虫启动前 max(id)）。
// crawler_run 节点存为 map[string]uint，经 JSON 往返后值类型变为 float64。
func (n *PlatformSyncNode) extractSourceBaselines(input map[string]interface{}) map[string]uint {
	val, ok := input["sourceMaxIdsBefore"]
	if !ok || val == nil {
		return nil
	}
	switch m := val.(type) {
	case map[string]uint:
		return m
	case map[string]interface{}:
		result := make(map[string]uint, len(m))
		for k, v := range m {
			switch id := v.(type) {
			case float64:
				result[k] = uint(id)
			case int:
				result[k] = uint(id)
			case int64:
				result[k] = uint(id)
			case uint:
				result[k] = id
			}
		}
		return result
	}
	return nil
}

// extractIncludeSourceIDs 从上游输入中提取过滤后的源表 ID 列表
func (n *PlatformSyncNode) extractIncludeSourceIDs(input map[string]interface{}) []uint {
	// 尝试从 includeSourceIds 字段获取
	if val, ok := input["includeSourceIds"]; ok {
		switch v := val.(type) {
		case []uint:
			return v
		case []interface{}:
			result := make([]uint, 0, len(v))
			for _, item := range v {
				switch id := item.(type) {
				case float64:
					result = append(result, uint(id))
				case int:
					result = append(result, uint(id))
				case int64:
					result = append(result, uint(id))
				case uint:
					result = append(result, id)
				}
			}
			return result
		}
	}
	return nil
}
