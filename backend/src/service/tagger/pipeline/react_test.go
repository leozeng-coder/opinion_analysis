package pipeline

import (
	"context"
	"strings"
	"testing"
)

// chunk 模拟 deep_think.formatChunk 的输出格式（头部含 [id=... ]）。
func makeChunk(articleID int, title, body string) string {
	return "[id=" + itoa(articleID) + " platform=weibo type=content score=0.9 src=hybrid]\n标题：" + title + "\n正文：" + body
}

func TestStripMetaHeader(t *testing.T) {
	c := makeChunk(8617, "薇尔诗口碑", "评论区一片好评")
	got := stripMetaHeader(c)
	if strings.Contains(got, "id=8617") {
		t.Fatalf("元数据行未被剥离: %q", got)
	}
	if !strings.HasPrefix(got, "标题：") {
		t.Fatalf("剥离后应以标题开头, got %q", got)
	}
}

func TestExtractArticleID(t *testing.T) {
	if id := extractArticleID(makeChunk(8617, "t", "b")); id != 8617 {
		t.Fatalf("want 8617, got %d", id)
	}
	if id := extractArticleID("标题：无元数据"); id != 0 {
		t.Fatalf("want 0, got %d", id)
	}
}

// 复现并验证修复：当 LLM 把文章 id（如 8617）当成序号返回时，
// 全部 item 越界 → validMapped==0 → 应保留全部候选，而非全丢。
func TestScoreAndAccumulate_IDCollisionFallback(t *testing.T) {
	chunks := []string{
		makeChunk(8617, "薇尔诗A", "好评"),
		makeChunk(9001, "薇尔诗B", "差评"),
	}
	// 模拟 LLM 返回文章 id 当序号（越界），全部无法映射
	badLLM := func(ctx context.Context, msgs []map[string]string) (string, error) {
		return `[{"id":8617,"score":5,"extract":"x"},{"id":9001,"score":4,"extract":"y"}]`, nil
	}
	n := NewReActNode(nil, badLLM)
	state := &PipelineState{UserQuestion: "薇尔诗口碑", Intent: "查询品牌口碑"}
	seen := map[int]struct{}{}

	kept := n.scoreAndAccumulate(context.Background(), state, chunks, 3, 1, seen)
	if kept != len(chunks) {
		t.Fatalf("ID 碰撞时应兜底保留全部 %d 条, got kept=%d", len(chunks), kept)
	}
	if len(state.Observations) != len(chunks) {
		t.Fatalf("observations 应为 %d, got %d", len(chunks), len(state.Observations))
	}
}

// 正常路径：LLM 返回正确序号，按分数筛选。
func TestScoreAndAccumulate_NormalFiltering(t *testing.T) {
	chunks := []string{
		makeChunk(8617, "薇尔诗A", "高度相关"),
		makeChunk(9001, "薇尔诗B", "无关"),
	}
	goodLLM := func(ctx context.Context, msgs []map[string]string) (string, error) {
		return `[{"id":1,"score":5,"extract":"相关摘要"},{"id":2,"score":1,"extract":""}]`, nil
	}
	n := NewReActNode(nil, goodLLM)
	state := &PipelineState{UserQuestion: "薇尔诗口碑", Intent: "查询品牌口碑"}
	seen := map[int]struct{}{}

	kept := n.scoreAndAccumulate(context.Background(), state, chunks, 3, 1, seen)
	if kept != 1 {
		t.Fatalf("应只保留 1 条(score>=3), got kept=%d", kept)
	}
	if len(state.Observations) != 1 || state.Observations[0].ArticleID != 8617 {
		t.Fatalf("应保留 article 8617, got %+v", state.Observations)
	}
}

func TestClampTopK(t *testing.T) {
	cases := map[int]int{0: 8, -5: 8, 2: 4, 4: 4, 8: 8, 12: 12, 20: 12}
	for in, want := range cases {
		if got := clampTopK(in); got != want {
			t.Errorf("clampTopK(%d)=%d, want %d", in, got, want)
		}
	}
}

// 验证渐进式扩容：级别越高，检索量越大、筛选门槛越松。
func TestExpandParams(t *testing.T) {
	base := 8
	p0, m0 := expandParams(0, base)
	p1, m1 := expandParams(1, base)
	p2, m2 := expandParams(2, base)

	if p0 != base || m0 != 3 {
		t.Errorf("L0 应为 base=%d/门槛3, got %d/%d", base, p0, m0)
	}
	if !(p1 > p0 && m1 < m0) {
		t.Errorf("L1 应检索量更大、门槛更松, got p1=%d m1=%d vs p0=%d m0=%d", p1, m1, p0, m0)
	}
	if !(p2 >= p1 && m2 <= m1) {
		t.Errorf("L2 应不小于 L1, got p2=%d m2=%d vs p1=%d m1=%d", p2, m2, p1, m1)
	}
}
