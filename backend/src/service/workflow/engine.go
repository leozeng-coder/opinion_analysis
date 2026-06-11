package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/milvus"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/report"
	"opinion-analysis/src/service/tagger"
	"opinion-analysis/src/service/digest"
	actionNodes "opinion-analysis/src/service/workflow/nodes/action"
	controlNodes "opinion-analysis/src/service/workflow/nodes/control"
	crawlerNodes "opinion-analysis/src/service/workflow/nodes/crawler"
	"opinion-analysis/src/service/workflow/nodes"
	processorNodes "opinion-analysis/src/service/workflow/nodes/processor"
)

// ErrCancelled 表示工作流被用户主动取消
var ErrCancelled = errors.New("workflow cancelled")

// Engine 工作流执行引擎
type Engine struct {
	db     *gorm.DB
	store  *repository.Store
	logger *zap.Logger

	// 依赖的服务
	taggerSvc   *tagger.Service
	ragProc     *ragprocess.Manager
	milvusSyncer *milvus.Syncer
	alertEngine *alertengine.Engine
	reportSvc   *report.Service

	// 运行中执行的取消注册表：executionID → cancel func
	cancelMu    sync.Mutex
	cancelFuncs map[int64]context.CancelFunc

	// 当前正在执行的节点：executionID → []NodeExecutor（支持并发多节点）
	activeNodeMu sync.Mutex
	activeNodes  map[int64][]NodeExecutor
}

// NewEngine 创建工作流引擎
func NewEngine(
	db *gorm.DB,
	store *repository.Store,
	logger *zap.Logger,
	taggerSvc *tagger.Service,
	ragProc *ragprocess.Manager,
	milvusSyncer *milvus.Syncer,
	alertEngine *alertengine.Engine,
	reportSvc *report.Service,
) *Engine {
	engine := &Engine{
		db:           db,
		store:        store,
		logger:       logger,
		taggerSvc:    taggerSvc,
		ragProc:      ragProc,
		milvusSyncer: milvusSyncer,
		alertEngine:  alertEngine,
		reportSvc:    reportSvc,
		cancelFuncs:  make(map[int64]context.CancelFunc),
		activeNodes:  make(map[int64][]NodeExecutor),
	}

	// 注册所有节点
	engine.registerNodes()

	return engine
}

// registerNodes 注册所有节点执行器
func (e *Engine) registerNodes() {
	// 爬虫类节点
	MustRegisterNode(crawlerNodes.NewRunNode(e.db, e.store.Crawler, e.store.System))
	MustRegisterNode(crawlerNodes.NewScheduleNode(e.store.Crawler))
	MustRegisterNode(crawlerNodes.NewStatusNode(e.store.Crawler))
	MustRegisterNode(crawlerNodes.NewDataPatchNode(e.db))

	// 处理类节点
	MustRegisterNode(processorNodes.NewPlatformSyncNode(e.db))
	MustRegisterNode(processorNodes.NewDataFilterNode(e.db))
	MustRegisterNode(processorNodes.NewAITaggerNode(e.taggerSvc))
	MustRegisterNode(processorNodes.NewRAGVectorizeNode(e.store.RAG, e.milvusSyncer))
	MustRegisterNode(processorNodes.NewAlertEvaluateNode(e.alertEngine))
	MustRegisterNode(processorNodes.NewDigestGenerateNode(
		digest.NewGenerator(e.db, e.store.Digest, e.taggerSvc),
	))

	// 动作类节点
	MustRegisterNode(actionNodes.NewHTTPRequestNode())
	MustRegisterNode(actionNodes.NewAnalysisReportNode(e.reportSvc))

	// 控制流节点
	MustRegisterNode(controlNodes.NewDelayNode())
	MustRegisterNode(controlNodes.NewConditionNode())
}

// CancelExecution 请求取消指定执行。若执行不存在或已结束返回 false。
func (e *Engine) CancelExecution(executionID int64) bool {
	e.activeNodeMu.Lock()
	nodeList := append([]NodeExecutor(nil), e.activeNodes[executionID]...)
	e.activeNodeMu.Unlock()

	e.cancelMu.Lock()
	cancel, ok := e.cancelFuncs[executionID]
	e.cancelMu.Unlock()

	if !ok {
		return false
	}

	// 先通知所有活跃节点做清理
	for _, node := range nodeList {
		bgCtx := context.Background()
		node.OnCancel(bgCtx)
	}

	// 再 cancel context
	cancel()
	return true
}

func (e *Engine) registerCancel(executionID int64, cancel context.CancelFunc) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	e.cancelFuncs[executionID] = cancel
}

func (e *Engine) unregisterCancel(executionID int64) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	delete(e.cancelFuncs, executionID)
}

func (e *Engine) addActiveNode(executionID int64, node NodeExecutor) {
	e.activeNodeMu.Lock()
	defer e.activeNodeMu.Unlock()
	e.activeNodes[executionID] = append(e.activeNodes[executionID], node)
}

func (e *Engine) removeActiveNode(executionID int64, node NodeExecutor) {
	e.activeNodeMu.Lock()
	defer e.activeNodeMu.Unlock()
	list := e.activeNodes[executionID]
	for i, n := range list {
		if n == node {
			e.activeNodes[executionID] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

// Execute 执行工作流
func (e *Engine) Execute(ctx context.Context, workflowID int64, manualInput map[string]interface{}) (*model.WorkflowExecution, error) {
	// 1. 加载工作流定义
	workflow, err := e.store.Workflow.FindByID(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow: %w", err)
	}

	e.logger.Info("starting workflow execution",
		zap.Int64("workflowId", workflowID),
		zap.String("workflowName", workflow.Name))

	// 2. 查找最近创建的 running 状态的执行记录（由 Handler 创建）
	// 如果找不到，则创建新的执行记录
	execution, err := e.store.WorkflowExecution.FindLatestRunning(workflowID)
	if err != nil || execution == nil {
		execution = &model.WorkflowExecution{
			WorkflowID: workflowID,
			Status:     "running",
			StartedAt:  time.Now(),
		}
		if err := e.store.WorkflowExecution.Create(execution); err != nil {
			return nil, fmt.Errorf("failed to create execution record: %w", err)
		}
	}

	// 包装可取消 context，注册到 engine.cancelFuncs，结束后清理
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.registerCancel(execution.ID, cancel)
	defer e.unregisterCancel(execution.ID)

	// 3. 解析节点和边
	var nodeList []map[string]interface{}
	var edgeList []map[string]interface{}

	if err := json.Unmarshal(workflow.Nodes, &nodeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse nodes: "+err.Error())
		return nil, fmt.Errorf("failed to parse nodes: %w", err)
	}

	if err := json.Unmarshal(workflow.Edges, &edgeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse edges: "+err.Error())
		return nil, fmt.Errorf("failed to parse edges: %w", err)
	}

	// 4. 拓扑排序（按 wave 分层，同层可并发）
	waves, err := e.topologicalWaves(nodeList, edgeList)
	if err != nil {
		e.updateExecutionStatus(execution.ID, "failed", err.Error())
		return nil, err
	}

	// 5. 按 wave 执行节点（同一 wave 内并发）
	nodeOutputs := make(map[string]map[string]interface{})
	outgoingActive := make(map[string]bool)
	partialNodes := 0
	skippedNodes := 0

	// 预计算每个节点是否有入边
	hasIncoming := make(map[string]bool)
	for _, edge := range edgeList {
		if t, ok := edge["target"].(string); ok {
			hasIncoming[t] = true
		}
	}

	e.logger.Info("starting node execution",
		zap.Int("totalNodes", len(nodeList)),
		zap.Int("waves", len(waves)))

	for _, wave := range waves {
		// 检查取消
		if err := execCtx.Err(); err != nil {
			e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
			return execution, ErrCancelled
		}

		// 预处理 wave：分出需要真正执行的节点
		var toExecute []map[string]interface{}
		for _, node := range wave {
			nodeID := node["id"].(string)
			nodeType := e.resolveNodeType(node)

			if nodeType == "trigger" {
				nodeOutputs[nodeID] = manualInput
				outgoingActive[nodeID] = true
				continue
			}

			if hasIncoming[nodeID] && !hasActiveIncoming(nodeID, edgeList, outgoingActive) {
				outgoingActive[nodeID] = false
				skippedNodes++
				e.recordSkippedNode(execution.ID, nodeID)
				continue
			}

			toExecute = append(toExecute, node)
		}

		if len(toExecute) == 0 {
			continue
		}

		// 单节点直接串行执行
		if len(toExecute) == 1 {
			node := toExecute[0]
			nodeID := node["id"].(string)
			nodeType := e.resolveNodeType(node)
			input := e.collectInputs(nodeID, edgeList, nodeOutputs)

			output, err := e.executeNode(execCtx, execution.ID, node, input)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(execCtx.Err(), context.Canceled) {
					e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
					return execution, ErrCancelled
				}
				e.updateExecutionStatus(execution.ID, "failed", err.Error())
				return execution, err
			}

			if output != nil {
				if s, ok := output["status"].(string); ok && (s == "partial_success" || s == "partial") {
					partialNodes++
				}
			}
			nodeOutputs[nodeID] = output
			if nodeType == "condition" {
				outgoingActive[nodeID] = conditionPassed(output)
			} else {
				outgoingActive[nodeID] = true
			}
			continue
		}

		// 多节点并发执行
		type nodeResult struct {
			nodeID  string
			output  map[string]interface{}
			err     error
		}

		waveCtx, waveCancel := context.WithCancel(execCtx)
		results := make([]nodeResult, len(toExecute))
		var wg sync.WaitGroup

		for i, node := range toExecute {
			wg.Add(1)
			go func(idx int, n map[string]interface{}) {
				defer wg.Done()
				nID := n["id"].(string)
				input := e.collectInputs(nID, edgeList, nodeOutputs)
				output, err := e.executeNode(waveCtx, execution.ID, n, input)
				results[idx] = nodeResult{nodeID: nID, output: output, err: err}
				if err != nil {
					waveCancel()
				}
			}(i, node)
		}
		wg.Wait()
		waveCancel()

		// 处理并发结果
		for i, node := range toExecute {
			res := results[i]
			nodeType := e.resolveNodeType(node)

			if res.err != nil {
				if errors.Is(res.err, context.Canceled) || errors.Is(execCtx.Err(), context.Canceled) {
					e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
					return execution, ErrCancelled
				}
				e.updateExecutionStatus(execution.ID, "failed", res.err.Error())
				return execution, res.err
			}

			if res.output != nil {
				if s, ok := res.output["status"].(string); ok && (s == "partial_success" || s == "partial") {
					partialNodes++
				}
			}
			nodeOutputs[res.nodeID] = res.output
			if nodeType == "condition" {
				outgoingActive[res.nodeID] = conditionPassed(res.output)
			} else {
				outgoingActive[res.nodeID] = true
			}
		}
	}

	// 6. 更新执行状态
	finalStatus := "success"
	finalMsg := ""
	if partialNodes > 0 {
		finalStatus = "partial_success"
		finalMsg = fmt.Sprintf("%d 个节点存在部分失败，详见节点日志", partialNodes)
	}
	if skippedNodes > 0 {
		if finalMsg != "" {
			finalMsg += "；"
		}
		finalMsg += fmt.Sprintf("%d 个节点因条件分支被跳过", skippedNodes)
	}
	e.updateExecutionStatus(execution.ID, finalStatus, finalMsg)

	e.logger.Info("workflow execution completed",
		zap.Int64("workflowId", workflowID),
		zap.Int64("executionId", execution.ID),
		zap.String("status", finalStatus))

	return execution, nil
}

// executeNode 执行单个节点
func (e *Engine) executeNode(ctx context.Context, executionID int64, node map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	nodeID := node["id"].(string)
	nodeType := e.resolveNodeType(node)
	config, _ := node["config"].(map[string]interface{})

	e.logger.Info("executing node",
		zap.String("nodeId", nodeID),
		zap.String("nodeType", nodeType))

	// 创建节点执行记录
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	nodeExec := &model.WorkflowNodeExecution{
		ExecutionID: executionID,
		NodeID:      nodeID,
		Status:      "running",
		Input:       model.JSON(inputJSON),
		StartedAt:   time.Now(),
	}
	if err := e.store.WorkflowNodeExecution.Create(nodeExec); err != nil {
		return nil, fmt.Errorf("failed to create node execution record: %w", err)
	}

	// 获取节点执行器
	executor, err := GetNodeExecutor(nodeType)
	if err != nil {
		errorMsg := fmt.Sprintf("Node type '%s' not registered", nodeType)
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, nil)
		return nil, fmt.Errorf("[Node: %s, Type: %s] %s", nodeID, nodeType, errorMsg)
	}

	// 验证配置
	if err := executor.Validate(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid config: %s", err.Error())
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, nil)
		return nil, fmt.Errorf("[Node: %s, Type: %s] %s", nodeID, nodeType, errorMsg)
	}

	// 注册为当前活跃节点，用于转发 OnCancel
	e.addActiveNode(executionID, executor)
	defer e.removeActiveNode(executionID, executor)

	// 注入进度回调：节点内部调用 nodes.ProgressFunc(ctx)(msg) 即可追加进度到 output
	ctx = nodes.WithProgressFunc(ctx, func(msg string) {
		e.appendNodeProgress(nodeExec.ID, msg)
	})

	// 执行节点
	output, err := executor.Execute(ctx, config, input)
	if err != nil {
		errorMsg := err.Error()
		// 取消被识别为 cancelled 状态而非 failed
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			e.updateNodeExecutionStatus(nodeExec.ID, "cancelled", "用户取消", nil)
			return nil, ctx.Err()
		}
		// 即使失败，若节点返回了 output（如 crawler timeout 仍有元信息），也保存到 DB，
		// 以便「从此节点重跑」时能重建有效的上下文（syncPlatformCodes 等）。
		var failedOutput map[string]interface{}
		if output != nil {
			failedOutput = diffPayload(input, output)
		}
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, failedOutput)
		return nil, fmt.Errorf("[Node: %s, Type: %s] Execution failed: %w", nodeID, nodeType, err)
	}

	// 节点完成态：partial_success 时也作为「完成」入库，但状态明确标记出来
	nodeStatus := "success"
	if output != nil {
		if s, ok := output["status"].(string); ok && (s == "partial_success" || s == "partial") {
			nodeStatus = "partial_success"
		}
	}

	// 持久化时只记录相对 input 的增量，避免重复字段把日志撑大；
	// 下游节点拿到的仍然是完整 output。
	outputDelta := diffPayload(input, output)
	e.updateNodeExecutionStatus(nodeExec.ID, nodeStatus, "", outputDelta)

	e.logger.Info("node execution completed",
		zap.String("nodeId", nodeID),
		zap.String("nodeType", nodeType))

	return output, nil
}

// topologicalSort 拓扑排序（平铺列表，兼容 ExecuteFromNode 等旧路径）
func (e *Engine) topologicalSort(nodeList []map[string]interface{}, edgeList []map[string]interface{}) ([]map[string]interface{}, error) {
	waves, err := e.topologicalWaves(nodeList, edgeList)
	if err != nil {
		return nil, err
	}
	var flat []map[string]interface{}
	for _, wave := range waves {
		flat = append(flat, wave...)
	}
	return flat, nil
}

// topologicalWaves 按层级拓扑排序，返回 wave 分组（同一 wave 内节点可并发执行）
func (e *Engine) topologicalWaves(nodeList []map[string]interface{}, edgeList []map[string]interface{}) ([][]map[string]interface{}, error) {
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)
	nodeMap := make(map[string]map[string]interface{})

	for _, node := range nodeList {
		nodeID := node["id"].(string)
		nodeMap[nodeID] = node
		inDegree[nodeID] = 0
	}

	for _, edge := range edgeList {
		source := edge["source"].(string)
		target := edge["target"].(string)
		adjList[source] = append(adjList[source], target)
		inDegree[target]++
	}

	// Kahn 算法，按 wave 分层
	var queue []string
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	var waves [][]map[string]interface{}
	visited := 0

	for len(queue) > 0 {
		wave := make([]map[string]interface{}, 0, len(queue))
		for _, id := range queue {
			wave = append(wave, nodeMap[id])
		}
		waves = append(waves, wave)
		visited += len(queue)

		var nextQueue []string
		for _, current := range queue {
			for _, neighbor := range adjList[current] {
				inDegree[neighbor]--
				if inDegree[neighbor] == 0 {
					nextQueue = append(nextQueue, neighbor)
				}
			}
		}
		queue = nextQueue
	}

	if visited != len(nodeList) {
		return nil, fmt.Errorf("workflow contains circular dependency")
	}

	return waves, nil
}

// collectInputs 收集上游节点的输出，对 slice 类型字段做 append 合并（支持并发节点输出汇聚）
func (e *Engine) collectInputs(nodeID string, edgeList []map[string]interface{}, outputs map[string]map[string]interface{}) map[string]interface{} {
	input := make(map[string]interface{})

	for _, edge := range edgeList {
		if edge["target"].(string) == nodeID {
			sourceID := edge["source"].(string)
			if output, ok := outputs[sourceID]; ok {
				for k, v := range output {
					existing, exists := input[k]
					if !exists {
						input[k] = v
						continue
					}
					// 对 []interface{} 类型做 append 合并
					if existSlice, ok := existing.([]interface{}); ok {
						if newSlice, ok := v.([]interface{}); ok {
							input[k] = append(existSlice, newSlice...)
							continue
						}
					}
					// 对 []string 类型做 append 合并
					if existSlice, ok := existing.([]string); ok {
						if newSlice, ok := v.([]string); ok {
							input[k] = append(existSlice, newSlice...)
							continue
						}
					}
					// 对 map[string]interface{} 类型做 key-level merge
					if existMap, ok := existing.(map[string]interface{}); ok {
						if newMap, ok := v.(map[string]interface{}); ok {
							for mk, mv := range newMap {
								existMap[mk] = mv
							}
							input[k] = existMap
							continue
						}
					}
					// 标量：后写覆盖
					input[k] = v
				}
			}
		}
	}

	return input
}

// hasActiveIncoming 判断 nodeID 是否存在至少一条「有效」入边。
// 由于按拓扑序执行，所有上游节点此时都已处理完，outgoingActive 已记录其有效性。
func hasActiveIncoming(nodeID string, edgeList []map[string]interface{}, outgoingActive map[string]bool) bool {
	for _, edge := range edgeList {
		target, _ := edge["target"].(string)
		if target != nodeID {
			continue
		}
		source, _ := edge["source"].(string)
		if active, ok := outgoingActive[source]; ok && active {
			return true
		}
	}
	return false
}

// conditionPassed 从 condition 节点输出读取 conditionResult；缺省视为通过。
func conditionPassed(output map[string]interface{}) bool {
	if output == nil {
		return true
	}
	if v, ok := output["conditionResult"].(bool); ok {
		return v
	}
	return true
}

// recordSkippedNode 为被条件分支跳过的节点写一条 skipped 执行记录，便于前端展示。
func (e *Engine) recordSkippedNode(executionID int64, nodeID string) {
	now := time.Now()
	rec := &model.WorkflowNodeExecution{
		ExecutionID: executionID,
		NodeID:      nodeID,
		Status:      "skipped",
		Input:       model.JSON("{}"),
		Output:      model.JSON("{}"),
		ErrorMsg:    "上游条件不满足，节点被跳过",
		StartedAt:   now,
		FinishedAt:  &now,
	}
	if err := e.store.WorkflowNodeExecution.Create(rec); err != nil {
		e.logger.Warn("failed to record skipped node",
			zap.String("nodeId", nodeID),
			zap.Error(err))
	}
}

// diffPayload 返回 output 相对 input 的增量字段（新增或值发生变化）。
// 用于日志持久化，避免把上游字段重复存一遍。
func diffPayload(input, output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return map[string]interface{}{}
	}
	if input == nil {
		return output
	}
	delta := make(map[string]interface{}, len(output))
	for k, v := range output {
		if iv, ok := input[k]; !ok || !reflect.DeepEqual(iv, v) {
			delta[k] = v
		}
	}
	return delta
}

// ExecuteFromNode 从指定节点重跑工作流，前序节点输出从上次执行记录中重建，不重新执行。
// prevExecID 为参考的历史执行 ID，fromNodeID 及其下游将重新运行。
func (e *Engine) ExecuteFromNode(ctx context.Context, workflowID int64, fromNodeID string, prevExecID int64) (*model.WorkflowExecution, error) {
	workflow, err := e.store.Workflow.FindByID(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow: %w", err)
	}

	e.logger.Info("starting execute-from-node",
		zap.Int64("workflowId", workflowID),
		zap.String("fromNodeId", fromNodeID),
		zap.Int64("prevExecId", prevExecID))

	// 复用 handler 预先创建的 running 记录，或自行创建
	execution, err := e.store.WorkflowExecution.FindLatestRunning(workflowID)
	if err != nil || execution == nil {
		execution = &model.WorkflowExecution{
			WorkflowID: workflowID,
			Status:     "running",
			StartedAt:  time.Now(),
		}
		if err := e.store.WorkflowExecution.Create(execution); err != nil {
			return nil, fmt.Errorf("failed to create execution record: %w", err)
		}
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.registerCancel(execution.ID, cancel)
	defer e.unregisterCancel(execution.ID)

	var nodeList []map[string]interface{}
	var edgeList []map[string]interface{}
	if err := json.Unmarshal(workflow.Nodes, &nodeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse nodes: "+err.Error())
		return nil, fmt.Errorf("failed to parse nodes: %w", err)
	}
	if err := json.Unmarshal(workflow.Edges, &edgeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse edges: "+err.Error())
		return nil, fmt.Errorf("failed to parse edges: %w", err)
	}

	sortedNodes, err := e.topologicalSort(nodeList, edgeList)
	if err != nil {
		e.updateExecutionStatus(execution.ID, "failed", err.Error())
		return nil, err
	}

	// 加载前次执行的节点记录，重建前序节点输出
	prevNodeExecs, _ := e.store.WorkflowNodeExecution.ListByExecutionID(prevExecID)
	prevNodeMap := make(map[string]*model.WorkflowNodeExecution, len(prevNodeExecs))
	for i := range prevNodeExecs {
		prevNodeMap[prevNodeExecs[i].NodeID] = &prevNodeExecs[i]
	}

	nodeOutputs := make(map[string]map[string]interface{})
	outgoingActive := make(map[string]bool)

	for _, node := range sortedNodes {
		nodeID := node["id"].(string)
		if nodeID == fromNodeID {
			break // 后续节点在主循环中执行
		}
		nodeType := e.resolveNodeType(node)
		if nodeType == "trigger" {
			nodeOutputs[nodeID] = map[string]interface{}{}
			outgoingActive[nodeID] = true
			continue
		}
		prev, ok := prevNodeMap[nodeID]
		if !ok || prev.Status == "skipped" {
			// 无记录或被跳过：仍标记 outgoingActive=true（force 模式），
			// 让 fromNodeID 能正常开始执行。输出用空 map。
			outgoingActive[nodeID] = true
			nodeOutputs[nodeID] = map[string]interface{}{}
			continue
		}
		if prev.Status == "cancelled" || prev.Status == "failed" {
			errMsg := fmt.Sprintf("上游节点 %s 状态为 %s，无法重跑下游节点；请先修复上游节点或使用补数节点", nodeID, prev.Status)
			e.updateExecutionStatus(execution.ID, "failed", errMsg)
			return nil, fmt.Errorf(errMsg)
		}
		// fullOutput = merge(storedInput, storedDelta)
		var storedInput, storedDelta map[string]interface{}
		_ = json.Unmarshal(prev.Input, &storedInput)
		_ = json.Unmarshal(prev.Output, &storedDelta)
		full := make(map[string]interface{}, len(storedInput)+len(storedDelta))
		for k, v := range storedInput {
			full[k] = v
		}
		for k, v := range storedDelta {
			full[k] = v
		}
		nodeOutputs[nodeID] = full
		if nodeType == "condition" {
			outgoingActive[nodeID] = conditionPassed(full)
		} else {
			// Force 模式：只要有 DB 记录就视为有效（即使 status=failed），
			// 因为用户主动选择从此节点重跑，意味着认为前序数据可用。
			outgoingActive[nodeID] = true
		}
	}

	// 为前序节点写入 inherited 记录，确保本次 execution 可被后续「从此节点重跑」使用
	for _, node := range sortedNodes {
		nodeID := node["id"].(string)
		if nodeID == fromNodeID {
			break
		}
		nodeType := e.resolveNodeType(node)
		if nodeType == "trigger" {
			continue
		}
		prev := prevNodeMap[nodeID]
		if prev == nil {
			continue
		}
		inheritedExec := &model.WorkflowNodeExecution{
			ExecutionID: execution.ID,
			NodeID:      nodeID,
			Status:      "inherited",
			Input:       prev.Input,
			Output:      prev.Output,
			StartedAt:   time.Now(),
		}
		now := time.Now()
		inheritedExec.FinishedAt = &now
		_ = e.store.WorkflowNodeExecution.Create(inheritedExec)
	}

	// 预计算入边
	hasIncoming := make(map[string]bool)
	for _, edge := range edgeList {
		if t, ok := edge["target"].(string); ok {
			hasIncoming[t] = true
		}
	}

	partialNodes := 0
	skippedNodes := 0

	for _, node := range sortedNodes {
		nodeID := node["id"].(string)
		nodeType := e.resolveNodeType(node)

		// 跳过已重建（前序）的节点
		if _, done := nodeOutputs[nodeID]; done {
			continue
		}

		if err := execCtx.Err(); err != nil {
			e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
			return execution, ErrCancelled
		}

		if nodeType == "trigger" {
			nodeOutputs[nodeID] = map[string]interface{}{}
			outgoingActive[nodeID] = true
			continue
		}

		if hasIncoming[nodeID] && !hasActiveIncoming(nodeID, edgeList, outgoingActive) {
			outgoingActive[nodeID] = false
			skippedNodes++
			e.recordSkippedNode(execution.ID, nodeID)
			continue
		}

		input := e.collectInputs(nodeID, edgeList, nodeOutputs)
		output, err := e.executeNode(execCtx, execution.ID, node, input)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(execCtx.Err(), context.Canceled) {
				e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
				return execution, ErrCancelled
			}
			e.updateExecutionStatus(execution.ID, "failed", err.Error())
			return execution, err
		}

		if output != nil {
			if s, ok := output["status"].(string); ok && (s == "partial_success" || s == "partial") {
				partialNodes++
			}
		}

		nodeOutputs[nodeID] = output
		if nodeType == "condition" {
			outgoingActive[nodeID] = conditionPassed(output)
		} else {
			outgoingActive[nodeID] = true
		}
	}

	finalStatus := "success"
	finalMsg := ""
	if partialNodes > 0 {
		finalStatus = "partial_success"
		finalMsg = fmt.Sprintf("%d 个节点存在部分失败，详见节点日志", partialNodes)
	}
	if skippedNodes > 0 {
		if finalMsg != "" {
			finalMsg += "；"
		}
		finalMsg += fmt.Sprintf("%d 个节点因条件分支被跳过", skippedNodes)
	}
	e.updateExecutionStatus(execution.ID, finalStatus, finalMsg)

	e.logger.Info("execute-from-node completed",
		zap.Int64("workflowId", workflowID),
		zap.Int64("executionId", execution.ID),
		zap.String("status", finalStatus))

	return execution, nil
}

func (e *Engine) resolveNodeType(node map[string]interface{}) string {
	if t, ok := node["type"].(string); ok && t != "" && t != "custom" && t != "trigger" {
		return t
	}
	if st, ok := node["subType"].(string); ok && st != "" {
		return st
	}
	if t, ok := node["type"].(string); ok {
		return t
	}
	return ""
}

// updateExecutionStatus 更新执行状态
func (e *Engine) updateExecutionStatus(executionID int64, status, errorMsg string) {
	now := time.Now()
	execution := &model.WorkflowExecution{
		ID:         executionID,
		Status:     status,
		FinishedAt: &now,
		ErrorMsg:   errorMsg,
	}
	if err := e.store.WorkflowExecution.Update(execution); err != nil {
		e.logger.Error("failed to update execution status",
			zap.Int64("executionId", executionID),
			zap.String("status", status),
			zap.Error(err))
	} else {
		e.logger.Info("execution status updated",
			zap.Int64("executionId", executionID),
			zap.String("status", status))
	}
}

// updateNodeExecutionStatus 更新节点执行状态
func (e *Engine) updateNodeExecutionStatus(nodeExecID int64, status, errorMsg string, output map[string]interface{}) {
	// 保留执行过程中写入的 progress 字段（appendNodeProgress 会中途写入）
	var cur model.WorkflowNodeExecution
	if err := e.store.WorkflowNodeExecution.FindByID(nodeExecID, &cur); err == nil {
		var existing map[string]interface{}
		if len(cur.Output) > 0 {
			_ = json.Unmarshal(cur.Output, &existing)
		}
		if progress, ok := existing["progress"]; ok {
			if output == nil {
				output = make(map[string]interface{})
			}
			output["progress"] = progress
		}
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		e.logger.Error("failed to marshal output",
			zap.Int64("nodeExecId", nodeExecID),
			zap.Error(err))
		// 使用空对象作为fallback
		outputJSON = []byte("{}")
	}
	now := time.Now()
	nodeExec := &model.WorkflowNodeExecution{
		ID:         nodeExecID,
		Status:     status,
		Output:     model.JSON(outputJSON),
		ErrorMsg:   errorMsg,
		FinishedAt: &now,
	}
	if err := e.store.WorkflowNodeExecution.Update(nodeExec); err != nil {
		e.logger.Error("failed to update node execution status",
			zap.Int64("nodeExecId", nodeExecID),
			zap.String("status", status),
			zap.Error(err))
	}
}

// appendNodeProgress 追加一条进度消息到节点执行记录的 output.progress 数组
func (e *Engine) appendNodeProgress(nodeExecID int64, msg string) {
	var cur model.WorkflowNodeExecution
	if err := e.store.WorkflowNodeExecution.FindByID(nodeExecID, &cur); err != nil {
		return
	}
	var output map[string]interface{}
	if len(cur.Output) > 0 {
		_ = json.Unmarshal(cur.Output, &output)
	}
	if output == nil {
		output = make(map[string]interface{})
	}

	var lines []string
	if existing, ok := output["progress"].([]interface{}); ok {
		for _, v := range existing {
			if s, ok := v.(string); ok {
				lines = append(lines, s)
			}
		}
	}
	lines = append(lines, msg)
	output["progress"] = lines

	outputJSON, _ := json.Marshal(output)
	_ = e.store.WorkflowNodeExecution.UpdateOutput(&model.WorkflowNodeExecution{
		ID:     nodeExecID,
		Output: model.JSON(outputJSON),
	})
}
