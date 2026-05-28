package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opinion-analysis/src/repository"
	"time"
)

// CrawlerRunNode 执行爬虫任务节点
type CrawlerRunNode struct {
	repo *repository.CrawlerRepository
}

func NewCrawlerRunNode(repo *repository.CrawlerRepository) *CrawlerRunNode {
	return &CrawlerRunNode{repo: repo}
}

func (n *CrawlerRunNode) Type() string {
	return "crawler_run"
}

func (n *CrawlerRunNode) Validate(config map[string]interface{}) error {
	// spiders 是可选的，默认使用 broad-topic
	return nil
}

func (n *CrawlerRunNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 解析配置
	platforms, _ := config["platforms"].([]interface{})
	keywords, _ := config["keywords"].([]interface{})
	topics, _ := config["topics"].([]interface{})
	waitForCompletion, _ := config["waitForCompletion"].(bool)
	timeoutMinutes, _ := config["timeoutMinutes"].(float64)

	if timeoutMinutes == 0 {
		timeoutMinutes = 10
	}

	// 转换为字符串数组
	platformList := make([]string, 0)
	for _, p := range platforms {
		if str, ok := p.(string); ok {
			platformList = append(platformList, str)
		}
	}

	keywordList := make([]string, 0)
	for _, k := range keywords {
		if str, ok := k.(string); ok {
			keywordList = append(keywordList, str)
		}
	}

	topicList := make([]string, 0)
	for _, t := range topics {
		if str, ok := t.(string); ok {
			topicList = append(topicList, str)
		}
	}

	// 默认使用小红书
	if len(platformList) == 0 {
		platformList = []string{"xiaohongshu"}
	}

	log.Printf("[CrawlerRunNode] Crawler task configured: platforms=%v keywords=%v topics=%v", platformList, keywordList, topicList)

	// TODO: 这里应该调用实际的爬虫服务
	// 目前返回模拟结果，避免依赖 MediaCrawler
	output := map[string]interface{}{
		"platforms":       platformList,
		"keywords":        keywordList,
		"topics":          topicList,
		"status":          "simulated",
		"message":         fmt.Sprintf("Crawler task simulated for %d platforms", len(platformList)),
		"articlesCount":   0,
		"waitedCompletion": waitForCompletion,
	}

	log.Printf("[CrawlerRunNode] Crawler task completed (simulated)")

	return output, nil
}

// CrawlerScheduleNode 配置爬虫调度节点
type CrawlerScheduleNode struct {
	repo *repository.CrawlerRepository
}

func NewCrawlerScheduleNode(repo *repository.CrawlerRepository) *CrawlerScheduleNode {
	return &CrawlerScheduleNode{repo: repo}
}

func (n *CrawlerScheduleNode) Type() string {
	return "crawler_schedule"
}

func (n *CrawlerScheduleNode) Validate(config map[string]interface{}) error {
	spiderKey, ok := config["spiderKey"].(string)
	if !ok || spiderKey == "" {
		return fmt.Errorf("spiderKey is required")
	}

	// 验证 spiderKey 是否合法
	allowedKeys := map[string]bool{
		"broad-topic":    true,
		"deep-sentiment": true,
	}

	if !allowedKeys[spiderKey] {
		return fmt.Errorf("invalid spiderKey: %s (allowed: broad-topic, deep-sentiment)", spiderKey)
	}

	return nil
}

func (n *CrawlerScheduleNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 解析配置
	spiderKey, _ := config["spiderKey"].(string)
	intervalMinutes, _ := config["intervalMinutes"].(float64)
	enabled, _ := config["enabled"].(bool)

	if intervalMinutes == 0 {
		intervalMinutes = 60 // 默认1小时
	}

	enabledInt := int8(0)
	if enabled {
		enabledInt = 1
	}

	log.Printf("[CrawlerScheduleNode] Updating spider schedule: key=%s interval=%d enabled=%v", spiderKey, int(intervalMinutes), enabled)

	// 更新爬虫调度配置
	err := n.repo.UpdateSpiderConfig(spiderKey, int(intervalMinutes), enabledInt)
	if err != nil {
		return nil, fmt.Errorf("failed to update spider config: %w", err)
	}

	output := map[string]interface{}{
		"spiderKey":       spiderKey,
		"intervalMinutes": int(intervalMinutes),
		"enabled":         enabled,
		"status":          "updated",
	}

	return output, nil
}

// CrawlerStatusNode 检查爬虫运行状态节点
type CrawlerStatusNode struct {
	repo *repository.CrawlerRepository
}

func NewCrawlerStatusNode(repo *repository.CrawlerRepository) *CrawlerStatusNode {
	return &CrawlerStatusNode{repo: repo}
}

func (n *CrawlerStatusNode) Type() string {
	return "crawler_status"
}

func (n *CrawlerStatusNode) Validate(config map[string]interface{}) error {
	// 配置都是可选的
	return nil
}

func (n *CrawlerStatusNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 解析配置
	runID, _ := config["runID"].(float64)
	checkRecent, _ := config["checkRecent"].(bool)

	if runID > 0 {
		// 检查特定运行记录
		log.Printf("[CrawlerStatusNode] Checking crawler run status: runID=%d", uint(runID))

		runLog, err := n.repo.FindRunLogByID(uint(runID))
		if err != nil {
			return nil, fmt.Errorf("failed to find run log: %w", err)
		}

		var progressDetail map[string]interface{}
		if runLog.ProgressDetail != "" {
			json.Unmarshal([]byte(runLog.ProgressDetail), &progressDetail)
		}

		output := map[string]interface{}{
			"runID":          runLog.ID,
			"status":         runLog.Status,
			"progress":       runLog.Progress,
			"progressDetail": progressDetail,
			"message":        runLog.Message,
			"startedAt":      runLog.StartedAt,
			"finishedAt":     runLog.FinishedAt,
		}

		return output, nil
	}

	if checkRecent {
		// 检查最近的运行记录
		log.Printf("[CrawlerStatusNode] Checking recent crawler runs")

		list, total, err := n.repo.ListRunLogs(1, 10)
		if err != nil {
			return nil, fmt.Errorf("failed to list run logs: %w", err)
		}

		output := map[string]interface{}{
			"total":       total,
			"recentRuns":  list,
			"hasRunning":  false,
			"hasFailed":   false,
			"lastRunTime": time.Time{},
		}

		// 统计状态
		for _, run := range list {
			if run.Status == "running" {
				output["hasRunning"] = true
			}
			if run.Status == "failed" {
				output["hasFailed"] = true
			}
			if run.StartedAt.After(output["lastRunTime"].(time.Time)) {
				output["lastRunTime"] = run.StartedAt
			}
		}

		return output, nil
	}

	return map[string]interface{}{
		"status": "no_check_performed",
	}, nil
}
