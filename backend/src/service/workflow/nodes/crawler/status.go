package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/workflow/nodes"
)

// StatusNode 查询爬虫运行状态，并把 crawlerRunId 注入 payload 供下游使用
type StatusNode struct {
	*nodes.BaseNode
	crawlerRepo *repository.CrawlerRepository
}

func NewStatusNode(crawlerRepo *repository.CrawlerRepository) *StatusNode {
	return &StatusNode{
		BaseNode:    nodes.NewBaseNode("crawler_status"),
		crawlerRepo: crawlerRepo,
	}
}

func (n *StatusNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *StatusNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 优先从配置拿 runID，其次从上游 payload
	runID := uint(n.GetInt(config, "runID", 0))
	if runID == 0 {
		runID = nodes.GetUint(input, "crawlerRunId")
	}
	checkRecent, _ := config["checkRecent"].(bool)

	if runID > 0 {
		log.Printf("[CrawlerStatusNode] Checking run status: runId=%d", runID)
		runLog, err := n.crawlerRepo.FindRunLogByID(runID)
		if err != nil {
			return nil, n.WrapError("find run log failed", fmt.Errorf("runId=%d: %w", runID, err))
		}

		var progressDetail map[string]interface{}
		if runLog.ProgressDetail != "" {
			_ = json.Unmarshal([]byte(runLog.ProgressDetail), &progressDetail)
		}

		produced := map[string]interface{}{
			"crawlerRunId":    runLog.ID,
			"crawlerStatus":   runLog.Status,
			"crawlerProgress": runLog.Progress,
			"crawlerMessage":  runLog.Message,
			"progressDetail":  progressDetail,
			"crawlerFinished": runLog.Status == "success" || runLog.Status == "failed",
		}
		return nodes.CarryForward(input, produced), nil
	}

	if checkRecent {
		log.Printf("[CrawlerStatusNode] Checking recent crawler runs")
		list, total, err := n.crawlerRepo.ListRunLogs(1, 10)
		if err != nil {
			return nil, n.WrapError("list run logs failed", err)
		}

		hasRunning, hasFailed := false, false
		var lastRunTime time.Time
		for _, run := range list {
			if run.Status == "running" {
				hasRunning = true
			}
			if run.Status == "failed" {
				hasFailed = true
			}
			if run.StartedAt.After(lastRunTime) {
				lastRunTime = run.StartedAt
			}
		}

		produced := map[string]interface{}{
			"crawlerTotal":      total,
			"crawlerHasRunning": hasRunning,
			"crawlerHasFailed":  hasFailed,
			"crawlerLastRunAt":  lastRunTime.Format(time.RFC3339),
			"crawlerRecentRuns": len(list),
		}
		return nodes.CarryForward(input, produced), nil
	}

	return nodes.CarryForward(input, map[string]interface{}{
		"crawlerStatus": "no_check_performed",
	}), nil
}
