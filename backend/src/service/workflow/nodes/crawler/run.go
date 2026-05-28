package crawler

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/repository"
	platformSync "opinion-analysis/src/service"
	crawlerSvc "opinion-analysis/src/service/crawler"
	"opinion-analysis/src/service/workflow/nodes"
)

// RunNode 执行爬虫任务，仅负责触发 MediaCrawler 并记录源表 baseline，
// 不做数据同步（由下游 platform_sync 节点处理）。
type RunNode struct {
	*nodes.BaseNode
	db          *gorm.DB
	crawlerRepo *repository.CrawlerRepository
	crawlerSvc  *crawlerSvc.Service
}

func NewRunNode(db *gorm.DB, crawlerRepo *repository.CrawlerRepository) *RunNode {
	return &RunNode{
		BaseNode:    nodes.NewBaseNode("crawler_run"),
		db:          db,
		crawlerRepo: crawlerRepo,
		crawlerSvc:  crawlerSvc.NewService(crawlerRepo),
	}
}

func (n *RunNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *RunNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	platforms := n.GetStringSlice(config, "platforms")
	keywords := n.GetStringSlice(config, "keywords")
	topics := n.GetStringSlice(config, "topics")
	waitForCompletion := n.GetBool(config, "waitForCompletion", true)
	timeoutMinutes := n.GetInt(config, "timeoutMinutes", 10)

	if len(platforms) == 0 {
		platforms = []string{"zhihu"}
	}

	syncCodes := platformSync.ResolveSyncCodes(platforms)

	// 触发爬虫前，记录每个目标源表的 max(id)，
	// 下游 platform_sync 节点以此作为「本次新增」的主键 baseline。
	sourceMaxIDs := n.captureSourceBaselines(ctx, syncCodes)

	log.Printf("[CrawlerRunNode] Starting crawler: platforms=%v syncCodes=%v keywords=%v sourceBaselines=%v",
		platforms, syncCodes, keywords, sourceMaxIDs)

	result, err := n.crawlerSvc.Trigger(ctx, crawlerSvc.TriggerParams{
		Spiders:        platforms,
		Keywords:       keywords,
		Topics:         topics,
		TimeoutMinutes: timeoutMinutes,
	})
	if err != nil {
		return nil, n.WrapError("failed to trigger crawler", err)
	}

	log.Printf("[CrawlerRunNode] Crawler triggered: runId=%d", result.RunID)

	if waitForCompletion {
		timeout := time.Duration(timeoutMinutes) * time.Minute
		if err := n.crawlerSvc.WaitForCompletion(ctx, result.RunID, timeout); err != nil {
			return nil, n.WrapError("crawler execution failed", err)
		}
		log.Printf("[CrawlerRunNode] Crawler finished: runId=%d", result.RunID)
	}

	status := "triggered"
	if waitForCompletion {
		status = "completed"
	}

	produced := map[string]interface{}{
		"crawlerRunId":       result.RunID,
		"crawlerStartedAt":   result.StartedAt.Format(time.RFC3339),
		"platforms":          platforms,
		"syncPlatformCodes":  syncCodes,
		"sourceMaxIdsBefore": sourceMaxIDs, // 平台 syncCode → 触发爬虫前的源表 max(id)
		"keywords":           keywords,
		"topics":             topics,
		"status":             status,
		"waitedCompletion":   waitForCompletion,
	}

	return nodes.CarryForward(input, produced), nil
}

// captureSourceBaselines 查询每个平台对应源表当前的 max(id)
func (n *RunNode) captureSourceBaselines(ctx context.Context, syncCodes []string) map[string]uint {
	baselines := make(map[string]uint, len(syncCodes))
	for _, code := range syncCodes {
		table := platformSync.SyncCodeToSourceTable(code)
		if table == "" {
			log.Printf("[CrawlerRunNode] Unknown source table for syncCode=%s, baseline=0", code)
			baselines[code] = 0
			continue
		}
		var maxID uint
		if err := n.db.WithContext(ctx).Table(table).
			Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
			log.Printf("[CrawlerRunNode] Query max id of %s failed: %v (treat as 0)", table, err)
			baselines[code] = 0
			continue
		}
		baselines[code] = maxID
	}
	return baselines
}
