package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	platformSync "opinion-analysis/src/service"
	"opinion-analysis/src/service/workflow/nodes"
)

// PlatformSyncNode 将 MediaCrawler 平台表同步到 articles 中心表
type PlatformSyncNode struct {
	*nodes.BaseNode
	db          *gorm.DB
	crawlerRepo *repository.CrawlerRepository
}

// NewPlatformSyncNode 创建平台数据同步节点
func NewPlatformSyncNode(db *gorm.DB, crawlerRepo *repository.CrawlerRepository) *PlatformSyncNode {
	return &PlatformSyncNode{
		BaseNode:    nodes.NewBaseNode("platform_sync"),
		db:          db,
		crawlerRepo: crawlerRepo,
	}
}

func (n *PlatformSyncNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *PlatformSyncNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	syncMode := n.GetString(config, "syncMode", "incremental")
	enableSentiment := n.GetBool(config, "enableSentiment", false)

	// 平台：优先节点配置，其次上游 crawler_run 输出
	platformKeys := n.GetStringSlice(config, "platforms")
	if len(platformKeys) == 0 {
		platformKeys = n.getUpstreamPlatforms(input)
	}
	syncCodes := platformSync.ResolveSyncCodes(platformKeys)
	if len(syncCodes) == 0 {
		syncCodes = platformSync.ResolveSyncCodes([]string{"zhihu"})
	}

	since, sinceSource, err := n.resolveSince(config, input, syncMode)
	if err != nil {
		return nil, n.WrapError("resolve sync time range failed", err)
	}

	log.Printf("[PlatformSyncNode] Syncing platforms=%v mode=%s since=%v (%s)",
		syncCodes, syncMode, since, sinceSource)

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

	articleIDs, err := n.listArticleIDs(ctx, syncCodes, since, syncMode)
	if err != nil {
		return nil, n.WrapError("query synced article ids failed", err)
	}

	output := n.MergeOutput(input, map[string]interface{}{
		"articleIds":    int64SliceToIface(articleIDs),
		"articlesCount": len(articleIDs),
		"syncMode":      syncMode,
		"syncPlatforms": syncCodes,
		"syncResults":   results,
		"syncNewCount":  totalNew,
		"syncSince":     since.Format(time.RFC3339),
		"status":        "synced",
	})

	log.Printf("[PlatformSyncNode] Sync completed: %d new records, %d article ids for downstream",
		totalNew, len(articleIDs))

	return output, nil
}

func (n *PlatformSyncNode) getUpstreamPlatforms(input map[string]interface{}) []string {
	if val, ok := input["platforms"].([]interface{}); ok {
		out := make([]string, 0, len(val))
		for _, v := range val {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if val, ok := input["platforms"].([]string); ok {
		return val
	}
	return nil
}

func (n *PlatformSyncNode) resolveSince(config, input map[string]interface{}, syncMode string) (time.Time, string, error) {
	if syncMode == "full" {
		return time.Time{}, "full", nil
	}

	// 上游 crawler_run 提供了 runId → 从运行日志取开始时间
	if runID := n.getCrawlerRunID(input); runID > 0 {
		runLog, err := n.crawlerRepo.FindRunLogByID(runID)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("crawler run %d not found: %w", runID, err)
		}
		return runLog.StartedAt.Add(-1 * time.Minute), "crawlerRunId", nil
	}

	// 配置指定回溯分钟数
	if minutes := n.GetInt(config, "syncSinceMinutes", 0); minutes > 0 {
		return time.Now().Add(-time.Duration(minutes) * time.Minute), "syncSinceMinutes", nil
	}

	// 默认：各平台使用系统记录的最后同步时间（SyncSinglePlatform 行为）
	return time.Time{}, "lastSyncTime", nil
}

func (n *PlatformSyncNode) getCrawlerRunID(input map[string]interface{}) uint {
	switch v := input["crawlerRunId"].(type) {
	case float64:
		return uint(v)
	case int:
		return uint(v)
	case int64:
		return uint(v)
	case uint:
		return v
	}
	return 0
}

func (n *PlatformSyncNode) listArticleIDs(ctx context.Context, syncCodes []string, since time.Time, syncMode string) ([]int64, error) {
	articlePlatforms := platformSync.ResolveArticlePlatforms(syncCodes)
	query := n.db.WithContext(ctx).Model(&model.Article{}).Where("platform IN ?", articlePlatforms)
	if syncMode != "full" && !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}

	var ids []int64
	if err := query.Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func int64SliceToIface(ids []int64) []interface{} {
	out := make([]interface{}, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
