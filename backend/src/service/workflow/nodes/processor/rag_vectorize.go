package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"opinion-analysis/config"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/workflow/nodes"
)

// RAGVectorizeNode 触发 RAG 服务对未向量化的文章做增量同步。
// 流程：EnsureStarted → 等待 /health → 创建 RagSyncLog → POST /v1/sync → 可选轮询完成。
type RAGVectorizeNode struct {
	*nodes.BaseNode
	ragRepo *repository.RAGRepository
	ragProc *ragprocess.Manager
}

func NewRAGVectorizeNode(ragRepo *repository.RAGRepository, ragProc *ragprocess.Manager) *RAGVectorizeNode {
	return &RAGVectorizeNode{
		BaseNode: nodes.NewBaseNode("rag_vectorize"),
		ragRepo:  ragRepo,
		ragProc:  ragProc,
	}
}

func (n *RAGVectorizeNode) Validate(config map[string]interface{}) error {
	return nil
}

func (n *RAGVectorizeNode) Execute(ctx context.Context, cfg map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	waitForCompletion := n.GetBool(cfg, "waitForCompletion", true)
	timeoutMinutes := n.GetInt(cfg, "timeoutMinutes", 5)
	onlyProvided := n.GetBool(cfg, "onlyProvidedIds", true)

	articleIDs := n.GetArticleIDs(input)

	if config.Cfg == nil || !config.Cfg.RAG.Enabled {
		log.Printf("[RAGVectorizeNode] rag.enabled=false, skip")
		return nodes.CarryForward(input, map[string]interface{}{
			"ragStatus":  "skipped",
			"ragMessage": "RAG disabled in config",
		}), nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL), "/")
	if baseURL == "" {
		return nil, n.WrapError("rag config", fmt.Errorf("embedding_service_url not configured"))
	}

	if onlyProvided && len(articleIDs) == 0 {
		log.Printf("[RAGVectorizeNode] No upstream articleIds, skip (onlyProvidedIds=true)")
		return nodes.CarryForward(input, map[string]interface{}{
			"ragStatus":  "skipped",
			"ragMessage": "no upstream articleIds",
		}), nil
	}

	// 1. 确保 RAG 进程在跑
	if err := n.ragProc.EnsureStarted(ctx); err != nil {
		return nil, n.WrapError("RAG service start failed", err)
	}

	// 2. 等待 /health 就绪（最多 30s）
	if !n.waitHealth(ctx, baseURL, 30*time.Second) {
		return nil, n.WrapError("RAG service health check failed",
			fmt.Errorf("service not ready at %s", baseURL))
	}

	// 3. 创建 sync log 记录
	logRow := model.RagSyncLog{
		Status:    "running",
		Progress:  0,
		Mode:      "workflow",
		StartedAt: time.Now(),
	}
	if err := n.ragRepo.CreateSyncLog(&logRow); err != nil {
		return nil, n.WrapError("create rag sync log failed", err)
	}

	// 4. POST /v1/sync 触发同步
	payload := map[string]interface{}{
		"sync_log_id": logRow.ID,
		"async":       true,
	}
	payloadBytes, _ := json.Marshal(payload)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/v1/sync", bytes.NewReader(payloadBytes))
	if err != nil {
		_ = n.ragRepo.FailSyncLog(&logRow, err.Error())
		return nil, n.WrapError("build /v1/sync request failed", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		_ = n.ragRepo.FailSyncLog(&logRow, err.Error())
		return nil, n.WrapError("call /v1/sync failed", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// 409 表示已有同步任务在跑——视为成功（让那个任务消费这次的新文章）
		log.Printf("[RAGVectorizeNode] /v1/sync busy (409): %s", string(body))
		_ = n.ragRepo.FailSyncLog(&logRow, "skipped: another sync running")
		return nodes.CarryForward(input, map[string]interface{}{
			"ragSyncLogId": logRow.ID,
			"ragStatus":    "skipped",
			"ragMessage":   "another sync already running",
		}), nil
	}
	if resp.StatusCode/100 != 2 {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		_ = n.ragRepo.FailSyncLog(&logRow, msg)
		return nil, n.WrapError("RAG sync rejected", fmt.Errorf("%s", msg))
	}

	log.Printf("[RAGVectorizeNode] /v1/sync accepted: syncLogId=%d, body=%s", logRow.ID, string(body))

	if !waitForCompletion {
		return nodes.CarryForward(input, map[string]interface{}{
			"ragSyncLogId": logRow.ID,
			"ragStatus":    "running",
			"ragMessage":   "sync started asynchronously",
		}), nil
	}

	// 5. 轮询同步状态
	result, pollErr := n.waitSyncLog(ctx, logRow.ID, time.Duration(timeoutMinutes)*time.Minute)
	if pollErr != nil {
		return nil, n.WrapError("wait rag sync failed", pollErr)
	}

	status := result.Status
	if status == "failed" {
		return nil, n.WrapError("RAG sync failed", fmt.Errorf("%s", result.Message))
	}

	produced := map[string]interface{}{
		"ragSyncLogId":     logRow.ID,
		"ragStatus":        status,
		"ragArticlesDone":  result.ArticlesProcessed,
		"ragChunksUpserted": result.ChunksUpserted,
		"ragChunksDeleted": result.ChunksDeleted,
	}
	log.Printf("[RAGVectorizeNode] sync finished: syncLogId=%d status=%s processed=%d chunks=%d",
		logRow.ID, status, result.ArticlesProcessed, result.ChunksUpserted)

	return nodes.CarryForward(input, produced), nil
}

// waitHealth 轮询 /health，最多 timeout 时间
func (n *RAGVectorizeNode) waitHealth(ctx context.Context, baseURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(1 * time.Second):
		}
	}
	return false
}

// waitSyncLog 轮询 RagSyncLog 状态直到 success/failed 或超时
func (n *RAGVectorizeNode) waitSyncLog(ctx context.Context, logID uint, timeout time.Duration) (*model.RagSyncLog, error) {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for sync log %d", logID)
		}

		// 直接查 sync log（用 RAGRepository 的 DB 句柄）
		var row model.RagSyncLog
		if err := n.findSyncLog(logID, &row); err != nil {
			log.Printf("[RAGVectorizeNode] find sync log %d failed: %v", logID, err)
		} else {
			if row.Status == "success" || row.Status == "completed" {
				return &row, nil
			}
			if row.Status == "failed" {
				return &row, nil
			}
			log.Printf("[RAGVectorizeNode] sync log %d still %s progress=%d", logID, row.Status, row.Progress)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// findSyncLog 用 RAGRepository 内部的 db 查 sync log
// (RAGRepository 没有 FindByID，所以这里通过 ListSyncLogs 旁路 — 实际场景下应该加一个方法，
// 这里为了最小改动用 ListSyncLogs，logID 一般在前 10 条内)
func (n *RAGVectorizeNode) findSyncLog(logID uint, row *model.RagSyncLog) error {
	logs, _, err := n.ragRepo.ListSyncLogs(1, 20)
	if err != nil {
		return err
	}
	for _, l := range logs {
		if l.ID == logID {
			*row = l
			return nil
		}
	}
	return fmt.Errorf("sync log %d not found in recent 20", logID)
}
