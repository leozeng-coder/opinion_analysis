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
// 基于持久化偏移量做主键增量，只处理 id > offset 的新行。
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

	perPlatformSourceIDs := n.extractPerPlatformSourceIDs(input)
	sourceBaselines := n.extractSourceBaselines(input)

	var topic string
	if topics := nodes.GetStringSliceFromInput(input, "topics"); len(topics) > 0 {
		topic = topics[0]
	}

	log.Printf("[PlatformSyncNode] mode=%s syncCodes=%v topic=%s perPlatform=%d",
		syncMode, syncCodes, topic, len(perPlatformSourceIDs))

	syncSvc := platformSync.NewPlatformSyncService(n.db)
	results := make(map[string]interface{})
	totalNew := 0
	hasErrors := false
	var allInsertedIDs []int64

	for _, code := range syncCodes {
		var result *platformSync.SyncResult
		var syncErr error
		var strategy string

		// 确定该平台的源表 ID 列表
		codeSourceIDs := n.resolveSourceIDsForCode(code, perPlatformSourceIDs)

		switch {
		case len(codeSourceIDs) > 0:
			strategy = "filtered"
			result, syncErr = syncSvc.SyncPlatformBySourceIDsWithTopic(ctx, code, codeSourceIDs, topic, enableSentiment)
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
		"articleIds":         nodes.PackArticleIDs(allInsertedIDs),
		"articlesCount":     len(allInsertedIDs),
		"syncMode":          syncMode,
		"syncPlatforms":     syncCodes,
		"syncPlatformCodes": syncCodes,
		"syncResults":       results,
		"syncNewCount":      totalNew,
		"status":            status,
	}

	log.Printf("[PlatformSyncNode] done: newRecords=%d articleIds=%d", totalNew, len(allInsertedIDs))
	return nodes.CarryForward(input, produced), nil
}

// resolveSourceIDsForCode 返回指定平台应使用的源表 ID 列表。
// 如果上游有 perPlatform 数据，则只使用该平台对应的 ID；没有则返回 nil 走其他同步策略。
func (n *PlatformSyncNode) resolveSourceIDsForCode(code string, perPlatform map[string][]uint) []uint {
	if perPlatform == nil {
		return nil
	}
	if ids, ok := perPlatform[code]; ok {
		return ids
	}
	return nil
}

// extractSourceBaselines 从上游 crawler_run 节点提取各平台源表 baseline（爬虫启动前 max(id)）。
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
			if id, ok := toUint(v); ok {
				result[k] = id
			}
		}
		return result
	}
	return nil
}

// extractPerPlatformSourceIDs 从上游提取按平台分组的源表 ID map。
func (n *PlatformSyncNode) extractPerPlatformSourceIDs(input map[string]interface{}) map[string][]uint {
	val, ok := input["includeSourceIdsByPlatform"]
	if !ok || val == nil {
		return nil
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string][]uint, len(m))
	for code, v := range m {
		if ids := toUintSlice(v); len(ids) > 0 {
			result[code] = ids
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func toUint(v interface{}) (uint, bool) {
	switch id := v.(type) {
	case float64:
		return uint(id), true
	case int:
		return uint(id), true
	case int64:
		return uint(id), true
	case uint:
		return id, true
	}
	return 0, false
}

func toUintSlice(v interface{}) []uint {
	switch ids := v.(type) {
	case []uint:
		return ids
	case []interface{}:
		out := make([]uint, 0, len(ids))
		for _, item := range ids {
			if id, ok := toUint(item); ok {
				out = append(out, id)
			}
		}
		return out
	}
	return nil
}
