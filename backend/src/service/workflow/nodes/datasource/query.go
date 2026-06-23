package datasource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/workflow/nodes"
)

// QueryNode 数据查询节点 - 从数据库查询文章和评论，作为工作流起始数据源
type QueryNode struct {
	*nodes.BaseNode
	db *gorm.DB
}

func NewQueryNode(db *gorm.DB) *QueryNode {
	return &QueryNode{
		BaseNode: nodes.NewBaseNode("data_query"),
		db:       db,
	}
}

func (n *QueryNode) Validate(config map[string]interface{}) error {
	// 至少需要一个查询条件
	hasCondition := false
	if len(n.GetStringSlice(config, "topics")) > 0 {
		hasCondition = true
	}
	if len(n.GetStringSlice(config, "platforms")) > 0 {
		hasCondition = true
	}
	if len(n.GetStringSlice(config, "sentiments")) > 0 {
		hasCondition = true
	}
	if n.GetString(config, "keyword", "") != "" {
		hasCondition = true
	}
	if n.GetString(config, "startDate", "") != "" || n.GetString(config, "endDate", "") != "" {
		hasCondition = true
	}

	if !hasCondition {
		return fmt.Errorf("至少需要指定一个查询条件")
	}

	return nil
}

func (n *QueryNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	topics := n.GetStringSlice(config, "topics")
	platforms := n.GetStringSlice(config, "platforms")
	sentiments := n.GetStringSlice(config, "sentiments")
	keyword := n.GetString(config, "keyword", "")
	startDate := n.GetString(config, "startDate", "")
	endDate := n.GetString(config, "endDate", "")
	limit := n.GetInt(config, "limit", 1000) // 默认最多返回1000条
	orderBy := n.GetString(config, "orderBy", "published_at") // 排序字段：published_at, created_at
	orderDir := n.GetString(config, "orderDir", "desc") // 排序方向：asc, desc

	progress := nodes.ProgressFunc(ctx)
	progress("开始查询文章数据")

	result, err := n.queryArticles(ctx, topics, platforms, sentiments, keyword, startDate, endDate, limit, orderBy, orderDir)
	if err != nil {
		return nil, n.WrapError("数据查询失败", err)
	}

	progress(fmt.Sprintf("查询完成：共 %d 条记录", result["count"]))

	return result, nil
}

// queryArticles 查询文章数据
func (n *QueryNode) queryArticles(
	ctx context.Context,
	topics, platforms, sentiments []string,
	keyword, startDate, endDate string,
	limit int,
	orderBy, orderDir string,
) (map[string]interface{}, error) {
	query := n.db.Model(&model.Article{}).Where("1=1")

	// 话题过滤
	if len(topics) > 0 {
		query = query.Where("topic IN ?", topics)
	}

	// 平台过滤
	if len(platforms) > 0 {
		query = query.Where("platform IN ?", platforms)
	}

	// 感情过滤
	if len(sentiments) > 0 {
		query = query.Where("sentiment IN ?", sentiments)
	}

	// 关键词过滤
	if keyword != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 时间范围过滤
	if startDate != "" {
		query = query.Where("published_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("published_at <= ?", endDate)
	}

	// 排序
	orderClause := orderBy + " " + strings.ToUpper(orderDir)
	if orderBy != "published_at" && orderBy != "created_at" {
		orderClause = "published_at DESC" // 默认回退
	}
	query = query.Order(orderClause)

	// 限制数量
	if limit > 0 {
		query = query.Limit(limit)
	}

	// 执行查询
	var articles []model.Article
	if err := query.Find(&articles).Error; err != nil {
		return nil, err
	}

	// 提取ID列表
	articleIDs := make([]int64, len(articles))
	for i, article := range articles {
		articleIDs[i] = int64(article.ID)
	}

	return map[string]interface{}{
		"articleIds": articleIDs,
		"count":      len(articleIDs),
		"topics":     topics,
		"platforms":  platforms,
		"sentiments": sentiments,
		"keyword":    keyword,
		"startDate":  startDate,
		"endDate":    endDate,
		"queriedAt":  time.Now().Format(time.RFC3339),
	}, nil
}
