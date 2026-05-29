package crawler

import (
	"context"
	"fmt"
	"log"

	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/workflow/nodes"
)

// ScheduleNode 更新爬虫定时调度配置
type ScheduleNode struct {
	*nodes.BaseNode
	crawlerRepo *repository.CrawlerRepository
}

func NewScheduleNode(crawlerRepo *repository.CrawlerRepository) *ScheduleNode {
	return &ScheduleNode{
		BaseNode:    nodes.NewBaseNode("crawler_schedule"),
		crawlerRepo: crawlerRepo,
	}
}

func (n *ScheduleNode) Validate(config map[string]interface{}) error {
	spiderKey, ok := config["spiderKey"].(string)
	if !ok || spiderKey == "" {
		return fmt.Errorf("spiderKey is required")
	}
	allowed := map[string]bool{"broad-topic": true, "deep-sentiment": true}
	if !allowed[spiderKey] {
		return fmt.Errorf("invalid spiderKey: %s (allowed: broad-topic, deep-sentiment)", spiderKey)
	}
	return nil
}

func (n *ScheduleNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	spiderKey, _ := config["spiderKey"].(string)
	intervalMinutes, _ := config["intervalMinutes"].(float64)
	enabled, _ := config["enabled"].(bool)

	if intervalMinutes == 0 {
		intervalMinutes = 60
	}

	enabledInt := int8(0)
	if enabled {
		enabledInt = 1
	}

	log.Printf("[CrawlerScheduleNode] Updating spider: key=%s interval=%d enabled=%v", spiderKey, int(intervalMinutes), enabled)

	if err := n.crawlerRepo.UpdateSpiderConfig(spiderKey, int(intervalMinutes), enabledInt); err != nil {
		return nil, n.WrapError("failed to update spider config", err)
	}

	produced := map[string]interface{}{
		"spiderKey":       spiderKey,
		"intervalMinutes": int(intervalMinutes),
		"enabled":         enabled,
		"status":          "updated",
	}
	return nodes.CarryForward(input, produced), nil
}
