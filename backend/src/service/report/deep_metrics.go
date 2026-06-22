package report

import (
	"fmt"
	"strings"
	"sync"
)

// StageStat 单个流水线阶段的执行统计
type StageStat struct {
	Name      string // 阶段名（中文）
	Kind      string // ""=分析阶段（成功率口径）；"filter"=过滤阶段（输入→保留口径）
	Attempted int    // 分析：尝试处理的单元数；过滤：输入条目数
	Succeeded int    // 分析：成功解析/生成数；过滤：保留条目数
	Retried   int    // 触发重试的次数
	Note      string // 附加说明（如去噪/去重数、高低价值切分）
}

// Rate 返回成功率（0~1）
func (s StageStat) Rate() float64 {
	if s.Attempted <= 0 {
		return 0
	}
	return float64(s.Succeeded) / float64(s.Attempted)
}

// icon 根据成功率/保留情况返回状态图标
func (s StageStat) icon() string {
	switch {
	case s.Attempted == 0:
		return "—"
	case s.Succeeded == 0:
		return "✗"
	case s.Succeeded == s.Attempted:
		return "✓"
	default:
		// 过滤阶段：保留部分是正常预期（去噪本就该减少），用 ✓；分析阶段部分成功用 ⚠
		if s.Kind == "filter" {
			return "✓"
		}
		return "⚠"
	}
}

// Line 单行可读摘要。过滤阶段走「输入→保留」口径，分析阶段走「成功率」口径。
func (s StageStat) Line() string {
	if s.Kind == "filter" {
		var b strings.Builder
		pct := 0.0
		if s.Attempted > 0 {
			pct = float64(s.Succeeded) / float64(s.Attempted) * 100
		}
		b.WriteString(fmt.Sprintf("%s %s %d→%d（保留%.0f%%）", s.icon(), s.Name, s.Attempted, s.Succeeded, pct))
		if s.Note != "" {
			b.WriteString("，" + s.Note)
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s 成功率 %.0f%% (%d/%d)",
		s.icon(), s.Name, s.Rate()*100, s.Succeeded, s.Attempted))
	if s.Retried > 0 {
		b.WriteString(fmt.Sprintf("，重试 %d", s.Retried))
	}
	if s.Note != "" {
		b.WriteString("，" + s.Note)
	}
	return b.String()
}

// PipelineMetrics 线程安全的流水线指标收集器
type PipelineMetrics struct {
	mu    sync.Mutex
	stats []StageStat
}

// Record 记录一个阶段的统计结果
func (m *PipelineMetrics) Record(s StageStat) {
	m.mu.Lock()
	m.stats = append(m.stats, s)
	m.mu.Unlock()
}

// Report 生成多行总览（用于 progress 推送 + 控制台）
func (m *PipelineMetrics) Report() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.stats) == 0 {
		return "深度分析流水线：无阶段统计"
	}
	var b strings.Builder
	b.WriteString("深度分析流水线各环节成功率：\n")
	for _, s := range m.stats {
		b.WriteString("  " + s.Line() + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
