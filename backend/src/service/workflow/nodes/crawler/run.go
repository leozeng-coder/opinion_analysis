package crawler

import (
	"context"
	"log"
	"time"

	"opinion-analysis/src/repository"
	platformSync "opinion-analysis/src/service"
	crawlerSvc "opinion-analysis/src/service/crawler"
	"opinion-analysis/src/service/workflow/nodes"
)

// RunNode 执行爬虫任务，仅负责触发 MediaCrawler，不做数据同步
type RunNode struct {
	*nodes.BaseNode
	crawlerRepo *repository.CrawlerRepository
	crawlerSvc  *crawlerSvc.Service
}

func NewRunNode(crawlerRepo *repository.CrawlerRepository) *RunNode {
	return &RunNode{
		BaseNode:    nodes.NewBaseNode("crawler_run"),
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

	log.Printf("[CrawlerRunNode] Starting crawler: platforms=%v syncCodes=%v keywords=%v",
		platforms, syncCodes, keywords)

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
		"crawlerRunId":      result.RunID,
		"crawlerStartedAt":  result.StartedAt.Format(time.RFC3339),
		"platforms":         platforms,
		"syncPlatformCodes": syncCodes,
		"keywords":          keywords,
		"topics":            topics,
		"status":            status,
		"waitedCompletion":  waitForCompletion,
	}

	return nodes.CarryForward(input, produced), nil
}
