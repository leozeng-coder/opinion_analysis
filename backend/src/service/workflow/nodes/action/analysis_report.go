package action

import (
	"context"
	"fmt"

	"opinion-analysis/src/service/report"
	"opinion-analysis/src/service/workflow/nodes"
)

// AnalysisReportNode 分析报告节点
// 消费上游 articleIds + crawlerRunId，生成 Markdown 或 HTML 分析报告存入 Redis，
// 输出 reportId 供前端下载。
type AnalysisReportNode struct {
	*nodes.BaseNode
	reportSvc *report.Service
}

func NewAnalysisReportNode(reportSvc *report.Service) *AnalysisReportNode {
	return &AnalysisReportNode{
		BaseNode:  nodes.NewBaseNode("analysis_report"),
		reportSvc: reportSvc,
	}
}

func (n *AnalysisReportNode) Validate(config map[string]interface{}) error {
	format := n.GetString(config, "format", "markdown")
	if format != "markdown" && format != "html" {
		return fmt.Errorf("format must be 'markdown' or 'html'")
	}
	return nil
}

func (n *AnalysisReportNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	format := report.Format(n.GetString(config, "format", "markdown"))
	htmlTheme := n.GetString(config, "htmlTheme", "random")
	sampleSize := n.GetInt(config, "sampleSize", 8)
	maxGroups := n.GetInt(config, "maxGroups", 5)
	maxTopicCards := n.GetInt(config, "maxTopicCards", 8)
	commentSampleSize := n.GetInt(config, "commentSampleSize", 18)

	articleIDs := n.GetArticleIDs(input)
	if len(articleIDs) == 0 {
		return nil, n.WrapError("analysis_report requires articleIds from upstream (platform_sync)", nil)
	}

	crawlerRunID := nodes.GetUint(input, "crawlerRunId")
	platforms := nodes.GetStringSliceFromInput(input, "syncPlatformCodes")
	if len(platforms) == 0 {
		platforms = nodes.GetStringSliceFromInput(input, "platforms")
	}
	topics := nodes.GetStringSliceFromInput(input, "topics")

	progress := nodes.ProgressFunc(ctx)
	progress(fmt.Sprintf("生成 %s 报告：%d 篇文章，平台 %v", format, len(articleIDs), platforms))

	reportID, err := n.reportSvc.Generate(ctx, articleIDs, crawlerRunID, platforms, topics, format, htmlTheme, sampleSize, maxGroups, maxTopicCards, commentSampleSize)
	if err != nil {
		return nil, n.WrapError("report generation failed", err)
	}

	progress(fmt.Sprintf("报告生成完成：%s", reportID))

	produced := map[string]interface{}{
		"reportId":     reportID,
		"reportFormat": string(format),
		"reportUrl":    "/api/reports/" + reportID,
		"status":       "report_generated",
	}
	return nodes.CarryForward(input, produced), nil
}
