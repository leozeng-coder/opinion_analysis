package crawler

import (
	"context"
	"log"
	"time"

	"opinion-analysis/src/repository"
	crawlerSvc "opinion-analysis/src/service/crawler"
	"opinion-analysis/src/service/workflow/nodes"
)

// RunNode 执行爬虫任务节点
// 作为工作流的起点，输出本次爬取的文章ID列表
type RunNode struct {
	*nodes.BaseNode
	crawlerRepo *repository.CrawlerRepository
	crawlerSvc  *crawlerSvc.Service
}

// NewRunNode 创建爬虫执行节点
func NewRunNode(crawlerRepo *repository.CrawlerRepository) *RunNode {
	return &RunNode{
		BaseNode:    nodes.NewBaseNode("crawler_run"),
		crawlerRepo: crawlerRepo,
		crawlerSvc:  crawlerSvc.NewService(crawlerRepo),
	}
}

// Validate 验证配置
func (n *RunNode) Validate(config map[string]interface{}) error {
	// 所有参数都是可选的
	return nil
}

// Execute 执行爬虫任务
func (n *RunNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 解析配置
	platforms := n.GetStringSlice(config, "platforms")
	keywords := n.GetStringSlice(config, "keywords")
	topics := n.GetStringSlice(config, "topics")
	waitForCompletion := n.GetBool(config, "waitForCompletion", true)
	timeoutMinutes := n.GetInt(config, "timeoutMinutes", 10)

	// 默认使用小红书
	if len(platforms) == 0 {
		platforms = []string{"broad-topic"}
	}

	log.Printf("[CrawlerRunNode] Starting crawler task: platforms=%v, keywords=%v, topics=%v",
		platforms, keywords, topics)

	// 触发爬虫任务
	result, err := n.crawlerSvc.Trigger(ctx, crawlerSvc.TriggerParams{
		Spiders:        platforms,
		Keywords:       keywords,
		Topics:         topics,
		TimeoutMinutes: timeoutMinutes,
	})
	if err != nil {
		return nil, n.WrapError("failed to trigger crawler", err)
	}

	log.Printf("[CrawlerRunNode] Crawler task triggered: runId=%d", result.RunID)

	// 等待爬虫完成；平台表 → articles 同步由下游 platform_sync 节点负责
	if waitForCompletion {
		log.Printf("[CrawlerRunNode] Waiting for crawler completion (timeout: %d minutes)", timeoutMinutes)

		timeout := time.Duration(timeoutMinutes) * time.Minute
		err := n.crawlerSvc.WaitForCompletion(ctx, result.RunID, timeout)
		if err != nil {
			return nil, n.WrapError("crawler execution failed", err)
		}
		log.Printf("[CrawlerRunNode] Crawler completed: runId=%d", result.RunID)
	} else {
		log.Printf("[CrawlerRunNode] Crawler task started asynchronously (not waiting for completion)")
	}

	status := "triggered"
	if waitForCompletion {
		status = "completed"
	}
	output := map[string]interface{}{
		"crawlerRunId":     result.RunID,
		"platforms":        platforms,
		"keywords":         keywords,
		"topics":           topics,
		"status":           status,
		"waitedCompletion": waitForCompletion,
	}

	return output, nil
}
