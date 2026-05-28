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

// PlatformSyncNode 将 MediaCrawler 平台表同步到 articles 中心表（与爬虫节点解耦，仅消费标准 input）
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

	since, sinceSource, err := n.resolveSince(config, input, syncMode)
	if err != nil {
		return nil, n.WrapError("resolve sync time range failed", err)
	}

	articlePlatforms := platformSync.ResolveArticlePlatforms(syncCodes)
	maxIDBefore, err := n.maxArticleID(ctx, articlePlatforms)
	if err != nil {
		return nil, n.WrapError("query max article id failed", err)
	}

	log.Printf("[PlatformSyncNode] Syncing platforms=%v mode=%s since=%v (%s) maxArticleIdBefore=%d",
		syncCodes, syncMode, since, sinceSource, maxIDBefore)

	syncSvc := platformSync.NewPlatformSyncService(n.db)
	results := make(map[string]interface{})
	totalNew := 0

	for _, code := range syncCodes {
		var result *platformSync.SyncResult
		var syncErr error

		if syncMode == "full" || since.IsZero() {
			result, syncErr = syncSvc.SyncSinglePlatform(ctx, code)
		} else {
			result, syncErr = syncSvc.SyncPlatformSince(ctx, code, since, enableSentiment)
		}
		if syncErr != nil {
			return nil, n.WrapError(fmt.Sprintf("sync platform %s failed", code), syncErr)
		}

		totalNew += result.NewCount
		results[code] = map[string]interface{}{
			"newCount":     result.NewCount,
			"skippedCount": result.SkippedCount,
			"errorCount":   result.ErrorCount,
			"status":       result.Status,
		}
		log.Printf("[PlatformSyncNode] Platform %s: new=%d skipped=%d errors=%d",
			code, result.NewCount, result.SkippedCount, result.ErrorCount)
	}

	articleIDs, err := n.listNewArticleIDs(ctx, articlePlatforms, maxIDBefore)
	if err != nil {
		return nil, n.WrapError("query synced article ids failed", err)
	}

	log.Printf("[PlatformSyncNode] Sync completed: syncNew=%d articleIds=%d for downstream",
		totalNew, len(articleIDs))

	produced := map[string]interface{}{
		"articleIds":        nodes.PackArticleIDs(articleIDs),
		"articlesCount":     len(articleIDs),
		"syncMode":          syncMode,
		"syncPlatforms":     syncCodes,
		"syncPlatformCodes": syncCodes,
		"syncResults":       results,
		"syncNewCount":      totalNew,
		"status":            "synced",
	}
	if !since.IsZero() {
		produced["syncSince"] = since.Format(time.RFC3339)
	}

	output := nodes.CarryForward(input, produced)

	log.Printf("[PlatformSyncNode] Output payload: articleIds=%v (type=%T), articlesCount=%d",
		output["articleIds"], output["articleIds"], output["articlesCount"])

	return output, nil
}

func (n *PlatformSyncNode) resolveSince(config, input map[string]interface{}, syncMode string) (time.Time, string, error) {
	if syncMode == "full" {
		return time.Time{}, "full", nil
	}

	if t := nodes.GetTime(input, "crawlerStartedAt"); !t.IsZero() {
		return t.Add(-1 * time.Minute), "crawlerStartedAt", nil
	}

	if minutes := n.GetInt(config, "syncSinceMinutes", 0); minutes > 0 {
		return time.Now().Add(-time.Duration(minutes) * time.Minute), "syncSinceMinutes", nil
	}

	return time.Time{}, "lastSyncTime", nil
}

func (n *PlatformSyncNode) maxArticleID(ctx context.Context, articlePlatforms []string) (uint, error) {
	var maxID uint
	err := n.db.WithContext(ctx).Model(&model.Article{}).
		Where("platform IN ?", articlePlatforms).
		Select("COALESCE(MAX(id), 0)").
		Scan(&maxID).Error
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
