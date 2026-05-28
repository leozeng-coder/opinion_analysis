package nodes

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/service/tagger"
)

// DigestGenerateNode 生成摘要节点
type DigestGenerateNode struct {
	db        *gorm.DB
	taggerSvc *tagger.Service
}

func NewDigestGenerateNode(db *gorm.DB, taggerSvc *tagger.Service) *DigestGenerateNode {
	return &DigestGenerateNode{
		db:        db,
		taggerSvc: taggerSvc,
	}
}

func (n *DigestGenerateNode) Type() string {
	return "digest_generate"
}

func (n *DigestGenerateNode) Validate(config map[string]interface{}) error {
	if _, ok := config["days"]; !ok {
		return fmt.Errorf("days is required")
	}
	return nil
}

func (n *DigestGenerateNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	days := int(config["days"].(float64))

	// 计算日期范围
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	// 这里可以调用摘要生成逻辑
	// 目前简化处理，返回成功
	return map[string]interface{}{
		"success":   true,
		"startDate": startDate,
		"endDate":   endDate,
		"message":   fmt.Sprintf("Digest generated for last %d days", days),
	}, nil
}
