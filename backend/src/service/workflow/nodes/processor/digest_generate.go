package processor

import (
	"context"
	"log"
	"time"

	"opinion-analysis/src/service/digest"
	"opinion-analysis/src/service/workflow/nodes"
)

// DigestGenerateNode 触发每日摘要生成
// 依赖注入已初始化的 digest.Generator（含 db、redis、tagger）。
type DigestGenerateNode struct {
	*nodes.BaseNode
	gen *digest.Generator
}

func NewDigestGenerateNode(gen *digest.Generator) *DigestGenerateNode {
	return &DigestGenerateNode{
		BaseNode: nodes.NewBaseNode("digest_generate"),
		gen:      gen,
	}
}

func (n *DigestGenerateNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *DigestGenerateNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	days := n.GetInt(config, "days", 1)
	if days < 1 {
		days = 1
	}

	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")

	log.Printf("[DigestGenerateNode] Generating digest: %s ~ %s", startDate, endDate)

	if n.gen == nil {
		log.Printf("[DigestGenerateNode] no digest generator configured, skip")
		return nodes.CarryForward(input, map[string]interface{}{
			"digestGenerated": false,
			"digestMessage":   "digest generator not configured",
		}), nil
	}

	if err := n.gen.GenerateRecentDigest(ctx); err != nil {
		return nil, n.WrapError("digest generation failed", err)
	}

	produced := map[string]interface{}{
		"digestGenerated": true,
		"digestDays":      days,
		"digestStartDate": startDate,
		"digestEndDate":   endDate,
		"status":          "digest_done",
	}

	log.Printf("[DigestGenerateNode] Digest generated: %s ~ %s", startDate, endDate)
	return nodes.CarryForward(input, produced), nil
}
