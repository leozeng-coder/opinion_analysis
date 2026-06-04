package processor

import (
	"context"
	"log"
	"time"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/milvus"
	"opinion-analysis/src/service/workflow/nodes"
)

// RAGVectorizeNode 触发 RAG 向量同步（直接调用 Go Milvus Syncer）
type RAGVectorizeNode struct {
	*nodes.BaseNode
	ragRepo *repository.RAGRepository
	syncer  *milvus.Syncer
}

func NewRAGVectorizeNode(ragRepo *repository.RAGRepository, syncer *milvus.Syncer) *RAGVectorizeNode {
	return &RAGVectorizeNode{
		BaseNode: nodes.NewBaseNode("rag_vectorize"),
		ragRepo:  ragRepo,
		syncer:   syncer,
	}
}

func (n *RAGVectorizeNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *RAGVectorizeNode) Execute(ctx context.Context, cfg map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	onlyProvided := n.GetBool(cfg, "onlyProvidedIds", true)
	articleIDs := n.GetArticleIDs(input)

	if config.Cfg == nil || !config.Cfg.RAG.Enabled {
		log.Printf("[RAGVectorizeNode] rag.enabled=false, skip")
		return nodes.CarryForward(input, map[string]interface{}{
			"ragStatus":  "skipped",
			"ragMessage": "RAG disabled in config",
		}), nil
	}

	if onlyProvided && len(articleIDs) == 0 {
		log.Printf("[RAGVectorizeNode] No upstream articleIds, skip (onlyProvidedIds=true)")
		return nodes.CarryForward(input, map[string]interface{}{
			"ragStatus":  "skipped",
			"ragMessage": "no upstream articleIds",
		}), nil
	}

	// 1. 创建 sync log 记录
	logRow := model.RagSyncLog{
		Status:    "running",
		Progress:  0,
		Mode:      "workflow",
		StartedAt: time.Now(),
	}
	if err := n.ragRepo.CreateSyncLog(&logRow); err != nil {
		return nil, n.WrapError("create rag sync log failed", err)
	}

	// 2. 直接调用 Go Milvus Syncer 执行同步
	result, err := n.syncer.RunOnce(ctx, logRow.ID)
	if err != nil {
		_ = n.ragRepo.FailSyncLog(&logRow, err.Error())
		return nil, n.WrapError("RAG sync failed", err)
	}

	// 3. 提取结果（syncer.RunOnce 返回的字段名）
	articlesProcessed := 0  // 处理的文章总数（包括无变化跳过的）
	articlesUpserted := 0   // 实际向量化的文章数
	chunksUpserted := 0
	chunksDeleted := 0

	if v, ok := result["processed"].(int); ok {
		articlesProcessed = v
	}
	if v, ok := result["upserted"].(int); ok {
		articlesUpserted = v
	}
	if v, ok := result["chunks_upserted"].(int); ok {
		chunksUpserted = v
	}
	if v, ok := result["chunks_deleted"].(int); ok {
		chunksDeleted = v
	}

	log.Printf("[RAGVectorizeNode] sync finished: syncLogId=%d processed=%d upserted=%d chunks_up=%d chunks_del=%d",
		logRow.ID, articlesProcessed, articlesUpserted, chunksUpserted, chunksDeleted)

	produced := map[string]interface{}{
		"ragSyncLogId":         logRow.ID,
		"ragStatus":            "completed",
		"ragArticlesProcessed": articlesProcessed, // 处理的总数（含跳过）
		"ragArticlesUpserted":  articlesUpserted,  // 实际向量化数
		"ragChunksUpserted":    chunksUpserted,
		"ragChunksDeleted":     chunksDeleted,
	}

	return nodes.CarryForward(input, produced), nil
}
