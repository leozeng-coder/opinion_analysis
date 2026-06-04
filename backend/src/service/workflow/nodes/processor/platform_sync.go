package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
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

	// 从上游获取 topics 列表（爬虫节点传递），取第一个作为 topic
	var topic string
	if topics := nodes.GetStringSliceFromInput(input, "topics"); len(topics) > 0 {
		topic = topics[0]
	}

	articlePlatforms := platformSync.ResolveArticlePlatforms(syncCodes)
	maxArticleIDBefore, err := n.maxArticleID(ctx, articlePlatforms)
	if err != nil {
		return nil, n.WrapError("query max article id failed", err)
	}

	log.Printf("[PlatformSyncNode] mode=%s syncCodes=%v topic=%s includeSourceIDs=%d maxArticleIdBefore=%d",
		syncMode, syncCodes, topic, len(includeSourceIDs), maxArticleIDBefore)

	syncSvc := platformSync.NewPlatformSyncService(n.db)
	results := make(map[string]interface{})
	totalNew := 0
	hasErrors := false

	for _, code := range syncCodes {
		var result *platformSync.SyncResult
		var syncErr error
		var strategy string

		switch {
		case len(includeSourceIDs) > 0:
			// 优先级最高：如果上游传递了过滤后的源表 ID 列表，只同步这些 ID
			strategy = "filtered"
			result, syncErr = syncSvc.SyncPlatformBySourceIDsWithTopic(ctx, code, includeSourceIDs, topic, enableSentiment)
		case syncMode == "full":
			// 真正的全表扫描对账（按 origin_url 去重，幂等），并对齐偏移量
			strategy = "full"
			result, syncErr = syncSvc.SyncPlatformFullWithTopic(ctx, code, topic, enableSentiment)
		case syncSinceMinutes > 0:
			// 显式时间窗口覆盖：按「最近 N 分钟发帖」同步（不走偏移量）
			since := time.Now().Add(-time.Duration(syncSinceMinutes) * time.Minute)
			strategy = fmt.Sprintf("since=%dm", syncSinceMinutes)
			result, syncErr = syncSvc.SyncPlatformSinceWithTopic(ctx, code, since, topic, enableSentiment)
		default:
			// 推荐路径：基于持久化偏移量的主键增量，gap-free 且 O(新增)
			strategy = "offset"
			result, syncErr = syncSvc.SyncPlatformByOffsetWithTopic(ctx, code, topic, enableSentiment)
		}
		if syncErr != nil {
			return nil, n.WrapError(fmt.Sprintf("sync platform %s failed", code), syncErr)
		}

		totalNew += result.NewCount
		if result.ErrorCount > 0 {
			hasErrors = true
		}
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

	// 本次同步新增的 articles：id > 同步前的 max(id)
	articleIDs, err := n.listNewArticleIDs(ctx, articlePlatforms, maxArticleIDBefore)
	if err != nil {
		return nil, n.WrapError("query synced article ids failed", err)
	}

	status := "synced"
	if hasErrors {
		status = "partial_success"
	}

	produced := map[string]interface{}{
		"articleIds":         nodes.PackArticleIDs(articleIDs),
		"articlesCount":      len(articleIDs),
		"syncMode":           syncMode,
		"syncPlatforms":      syncCodes,
		"syncPlatformCodes":  syncCodes,
		"syncResults":        results,
		"syncNewCount":       totalNew,
		"maxArticleIdBefore": maxArticleIDBefore,
		"status":             status,
	}

	output := nodes.CarryForward(input, produced)

	log.Printf("[PlatformSyncNode] done: newRecords=%d articleIds=%d (%v)",
		totalNew, len(articleIDs), articleIDs)

	return output, nil
}

func (n *PlatformSyncNode) maxArticleID(ctx context.Context, articlePlatforms []string) (uint, error) {
	var maxID uint
	err := n.db.WithContext(ctx).Model(&model.Article{}).
		Where("platform IN ?", articlePlatforms).
		Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error
	return maxID, err
}

func (n *PlatformSyncNode) listNewArticleIDs(ctx context.Context, articlePlatforms []string, afterID uint) ([]int64, error) {
	var ids []int64
	err := n.db.WithContext(ctx).Model(&model.Article{}).
		Where("platform IN ? AND id > ?", articlePlatforms, afterID).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
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
