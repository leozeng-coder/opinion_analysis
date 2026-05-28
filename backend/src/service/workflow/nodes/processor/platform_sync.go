package processor

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	platformSync "opinion-analysis/src/service"
	"opinion-analysis/src/service/workflow/nodes"
)

// PlatformSyncNode 将 MediaCrawler 平台表同步到 articles 中心表。
// 关键设计：基于"源表主键 PK 增量"而非"源帖子发帖时间"，
// 由上游 crawler_run 节点提供 sourceMaxIdsBefore baseline，避免因源帖时间在过去导致漏数据。
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

	// 上游 crawler_run 提供的源表 baseline（主键增量起点）
	sourceBaselines := readSourceBaselines(input)

	articlePlatforms := platformSync.ResolveArticlePlatforms(syncCodes)
	maxArticleIDBefore, err := n.maxArticleID(ctx, articlePlatforms)
	if err != nil {
		return nil, n.WrapError("query max article id failed", err)
	}

	log.Printf("[PlatformSyncNode] mode=%s syncCodes=%v sourceBaselines=%v maxArticleIdBefore=%d",
		syncMode, syncCodes, sourceBaselines, maxArticleIDBefore)

	syncSvc := platformSync.NewPlatformSyncService(n.db)
	results := make(map[string]interface{})
	totalNew := 0
	hasErrors := false

	for _, code := range syncCodes {
		baseline := sourceBaselines[code]
		var result *platformSync.SyncResult
		var syncErr error
		var strategy string

		switch {
		case syncMode == "full":
			strategy = "full"
			result, syncErr = syncSvc.SyncSinglePlatform(ctx, code)
		case baseline > 0:
			// 推荐路径：基于源表 PK 增量
			strategy = fmt.Sprintf("sourceId>%d", baseline)
			result, syncErr = syncSvc.SyncPlatformFromSourceID(ctx, code, baseline, enableSentiment)
		default:
			// 无 baseline 时回退到「最后同步时间」策略
			strategy = "lastSyncTime"
			result, syncErr = syncSvc.SyncSinglePlatform(ctx, code)
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

// readSourceBaselines 从上游 input 读取 sourceMaxIdsBefore，
// 兼容 JSON 反序列化后的 map[string]interface{}{float64}
func readSourceBaselines(input map[string]interface{}) map[string]uint {
	out := make(map[string]uint)
	if input == nil {
		return out
	}
	raw, ok := input["sourceMaxIdsBefore"]
	if !ok || raw == nil {
		return out
	}
	switch m := raw.(type) {
	case map[string]uint:
		for k, v := range m {
			out[k] = v
		}
	case map[string]interface{}:
		for k, v := range m {
			switch n := v.(type) {
			case float64:
				out[k] = uint(n)
			case int:
				out[k] = uint(n)
			case int64:
				out[k] = uint(n)
			case uint:
				out[k] = n
			}
		}
	}
	return out
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
