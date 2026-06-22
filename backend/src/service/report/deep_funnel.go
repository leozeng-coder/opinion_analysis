package report

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"opinion-analysis/src/model"
)

// ── 漏斗式深度分析的过滤/排序原语（零或低成本，不调 LLM）──
//
// 设计目标：在昂贵的 LLM 挖掘之前，先用规则把「大量数据」做粗过滤（去噪/去重）
// 和细过滤（按廉价信号切分高/低决策价值），把算力集中到高价值数据上。
// 低价值数据降级保留——仍计入统计图表/趋势/兜底，信息不丢。

// commentStopwords 整条命中即视为无信息量噪音的评论（去标点/空白后精确匹配）。
var commentStopwords = map[string]struct{}{
	"好": {}, "好的": {}, "赞": {}, "点赞": {}, "顶": {}, "顶顶": {}, "支持": {},
	"沙发": {}, "前排": {}, "路过": {}, "围观": {}, "打卡": {}, "签到": {}, "mark": {}, "马克": {},
	"哈哈": {}, "哈哈哈": {}, "哈哈哈哈": {}, "呵呵": {}, "嘻嘻": {}, "嘿嘿": {},
	"666": {}, "6666": {}, "牛": {}, "牛逼": {}, "厉害": {}, "强": {}, "绝了": {},
	"不错": {}, "可以": {}, "正解": {}, "同意": {}, "赞同": {}, "说得对": {}, "说得好": {},
	"已阅": {}, "收到": {}, "知道了": {}, "了解": {}, "懂了": {}, "学习了": {}, "涨知识了": {},
	"第一": {}, "第二": {}, "占楼": {}, "顶起": {}, "顶上去": {},
}

// normalizeComment 去除标点、空白、emoji，转小写，用于去重 key 与停用词匹配。
func normalizeComment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// isNoiseComment 规则去噪：清洗后过短（<4 rune）或整条命中停用词即视为噪音。
func isNoiseComment(content string) bool {
	norm := normalizeComment(content)
	if norm == "" {
		return true
	}
	if len([]rune(norm)) < 4 {
		return true
	}
	if _, ok := commentStopwords[norm]; ok {
		return true
	}
	return false
}

// coarseFilterComments 评论粗过滤（漏斗①）：规则去噪 + 精确去重。
// 返回用于 LLM 分析的「清洗视图」和漏斗指标。
// 重要：原始 comments 不销毁——调用方的趋势/平台分布/计数仍用原始集（降级保留 volume）。
// 去重保留首现条（已按点赞排序时即最高赞），重复条数累加到该条 LikeCount 作为代表性权重。
func coarseFilterComments(comments []model.ArticleComment) ([]model.ArticleComment, StageStat) {
	stat := StageStat{Name: "粗过滤(评论)", Kind: "filter", Attempted: len(comments)}
	if len(comments) == 0 {
		return nil, stat
	}

	// 先按点赞降序，保证去重时保留的首现条是最高赞代表
	sorted := make([]model.ArticleComment, len(comments))
	copy(sorted, comments)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].LikeCount > sorted[j].LikeCount })

	seen := make(map[string]int, len(sorted)) // norm key → cleaned 切片下标
	cleaned := make([]model.ArticleComment, 0, len(sorted))
	var noise, dup int
	for _, c := range sorted {
		if isNoiseComment(c.Content) {
			noise++
			continue
		}
		key := normalizeComment(c.Content)
		if idx, ok := seen[key]; ok {
			// 重复：把它的点赞累加到代表条，体现真实热度
			cleaned[idx].LikeCount += c.LikeCount + 1
			dup++
			continue
		}
		seen[key] = len(cleaned)
		cleaned = append(cleaned, c)
	}

	stat.Succeeded = len(cleaned)
	stat.Note = noiseNote(noise, dup)
	return cleaned, stat
}

func noiseNote(noise, dup int) string {
	var parts []string
	if noise > 0 {
		parts = append(parts, "去噪 "+itoa(noise))
	}
	if dup > 0 {
		parts = append(parts, "去重 "+itoa(dup))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，")
}

// articleScore 文章的廉价决策价值信号（不调 LLM）：互动量 + 认同 + 时效 + 内容充分度。
func articleScore(a model.Article, comments []model.ArticleComment, refDate time.Time) float64 {
	commentCount := float64(len(comments))
	var likes float64
	for _, c := range comments {
		likes += float64(c.LikeCount)
	}
	ageDays := int(refDate.Sub(a.PublishedAt).Hours() / 24)
	tw := timeWeight(ageDays)
	contentLen := float64(len([]rune(a.Content)))

	// 经验加权：评论量权重最高（用户互动是决策价值最强信号），其次认同、内容、时效
	score := commentCount*3.0 + likes*0.05 + contentLen*0.01
	score *= (0.5 + tw) // 时效作为乘子，新内容加成、陈旧内容衰减但不归零
	return score
}

// selectHighValueArticles 文章细过滤（漏斗②）：按廉价信号切分高/低决策价值。
// 高价值进昂贵 LLM 挖掘；低价值降级走 fallback。保证有互动的内容绝不进 low。
func selectHighValueArticles(articles []model.Article, commentMap map[uint][]model.ArticleComment, refDate time.Time) (high, low []model.Article, stat StageStat) {
	stat = StageStat{Name: "细过滤(文章)", Kind: "filter", Attempted: len(articles)}
	n := len(articles)
	if n == 0 {
		return nil, nil, stat
	}

	type scored struct {
		a     model.Article
		score float64
		inter bool // 是否有互动（评论或点赞）
	}
	items := make([]scored, n)
	for i, a := range articles {
		cs := commentMap[a.ID]
		var likes int
		for _, c := range cs {
			likes += c.LikeCount
		}
		items[i] = scored{a: a, score: articleScore(a, cs, refDate), inter: len(cs) > 0 || likes > 0}
	}

	// 小数据集：不做激进 top-K，仅剔除「无互动 且 内容稀薄 且 陈旧」的项进 low
	const smallSet = 120
	if n <= smallSet {
		for _, it := range items {
			ageDays := int(refDate.Sub(it.a.PublishedAt).Hours() / 24)
			thin := !it.inter && len([]rune(it.a.Content)) < 50 && ageDays > 90
			if thin {
				low = append(low, it.a)
			} else {
				high = append(high, it.a)
			}
		}
		stat.Succeeded = len(high)
		stat.Note = highLowNote(len(high), len(low))
		return high, low, stat
	}

	// 大数据集：按分数 top-K 进 high，其余进 low；有互动的强制进 high
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	k := n * 6 / 10
	if k < 80 {
		k = 80
	}
	for i, it := range items {
		if i < k || it.inter {
			high = append(high, it.a)
		} else {
			low = append(low, it.a)
		}
	}
	stat.Succeeded = len(high)
	stat.Note = highLowNote(len(high), len(low))
	return high, low, stat
}

func highLowNote(high, low int) string {
	if low == 0 {
		return "全部高价值"
	}
	return "高价值 " + itoa(high) + "，降级 " + itoa(low)
}

// rankInsightsByValue 按决策价值降序排序洞察（漏斗④的排序原语）。
// 综合分 = DecisionValue × Endorsement × TimeWeight，与原簇内排序口径一致。
func rankInsightsByValue(insights []ArticleInsight) {
	sort.SliceStable(insights, func(i, j int) bool {
		si := float64(insights[i].DecisionValue) * (insights[i].Endorsement + 1) * (insights[i].TimeWeight + 0.1)
		sj := float64(insights[j].DecisionValue) * (insights[j].Endorsement + 1) * (insights[j].TimeWeight + 0.1)
		return si > sj
	})
}

// itoa 轻量 int→string，避免在多处 import strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
