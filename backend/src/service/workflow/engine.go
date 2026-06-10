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

	// 当前正在执行的节点：executionID → NodeExecutor（用于转发 OnCancel）
	activeNodeMu sync.Mutex
	activeNodes  map[int64]NodeExecutor
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
		activeNodes:  make(map[int64]NodeExecutor),
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
// 取消分两步：
//  1. 调用注册的 cancel func 取消 context（所有依赖 ctx.Done 的阻塞点会退出）
//  2. 通知当前正在执行的节点调用其 OnCancel（用于需要主动停止外部资源的节点，如爬虫）
func (e *Engine) CancelExecution(executionID int64) bool {
	// 先拿到当前节点，再 cancel（避免节点在 cancel 后还未读到 activeNodes）
	e.activeNodeMu.Lock()
	node, hasNode := e.activeNodes[executionID]
	e.activeNodeMu.Unlock()

	e.cancelMu.Lock()
	cancel, ok := e.cancelFuncs[executionID]
	e.cancelMu.Unlock()

	if !ok {
		return false
	}

	// 先通知节点做清理（此时 ctx 还未 Done，节点内部可以做最后的操作）
	if hasNode {
		bgCtx := context.Background()
		node.OnCancel(bgCtx)
	}

	// 再 cancel context，让阻塞的 WaitForCompletion / 轮询 退出
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

func (e *Engine) setActiveNode(executionID int64, node NodeExecutor) {
	e.activeNodeMu.Lock()
	defer e.activeNodeMu.Unlock()
	e.activeNodes[executionID] = node
}

func (e *Engine) clearActiveNode(executionID int64) {
	e.activeNodeMu.Lock()
	defer e.activeNodeMu.Unlock()
	delete(e.activeNodes, executionID)
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

	// 4. 拓扑排序（确定执行顺序）
	sortedNodes, err := e.topologicalSort(nodeList, edgeList)
	if err != nil {
		e.updateExecutionStatus(execution.ID, "failed", err.Error())
		return nil, err
	}

	// 5. 按顺序执行节点
	nodeOutputs := make(map[string]map[string]interface{})
	// outgoingActive[nodeID] 标记该节点的出边是否「有效」：
	//   - 普通节点执行成功 → true
	//   - condition 节点 → 取 conditionResult
	//   - 被跳过的节点 → false
	// 下游节点只有在「至少一条入边有效」时才会执行，从而实现条件分支。
	outgoingActive := make(map[string]bool)
	partialNodes := 0 // 节点 success 但内部有错误（partial_success）的计数
	skippedNodes := 0 // 因上游条件不满足而被跳过的节点计数

	// 预计算每个节点是否有入边（无入边的起点节点始终执行）
	hasIncoming := make(map[string]bool)
	for _, edge := range edgeList {
		if t, ok := edge["target"].(string); ok {
			hasIncoming[t] = true
		}
	}

	e.logger.Info("starting node execution",
		zap.Int("totalNodes", len(sortedNodes)))

	for _, node := range sortedNodes {
		nodeID := node["id"].(string)
		nodeType := e.resolveNodeType(node)

		e.logger.Info("processing node",
			zap.String("nodeId", nodeID),
			zap.String("nodeType", nodeType))

		// 在每个节点开始前检查取消
		if err := execCtx.Err(); err != nil {
			e.logger.Warn("workflow cancelled before node", zap.String("nodeId", nodeID))
			e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
			return execution, ErrCancelled
		}

		// 跳过 trigger 节点（trigger 只是起点标记）
		if nodeType == "trigger" {
			e.logger.Info("skipping trigger node", zap.String("nodeId", nodeID))
			nodeOutputs[nodeID] = manualInput
			outgoingActive[nodeID] = true
			continue
		}

		// 条件分支：若该节点有入边但没有任何一条「有效」入边，则跳过
		// （上游 condition 判否，或上游本身被跳过）。
		if hasIncoming[nodeID] && !hasActiveIncoming(nodeID, edgeList, outgoingActive) {
			e.logger.Info("skipping node: no active upstream branch",
				zap.String("nodeId", nodeID),
				zap.String("nodeType", nodeType))
			outgoingActive[nodeID] = false
			skippedNodes++
			e.recordSkippedNode(execution.ID, nodeID)
			continue
		}

		// 获取上游节点的输出作为输入
		input := e.collectInputs(nodeID, edgeList, nodeOutputs)

		e.logger.Info("executing node",
			zap.String("nodeId", nodeID),
			zap.String("nodeType", nodeType),
			zap.Any("input", input),
			zap.Int("inputKeys", len(input)))

		// 执行节点
		output, err := e.executeNode(execCtx, execution.ID, node, input)
		if err != nil {
			// 区分「取消」与「失败」
			if errors.Is(err, context.Canceled) || errors.Is(execCtx.Err(), context.Canceled) {
				e.logger.Warn("node cancelled", zap.String("nodeId", nodeID))
				e.updateExecutionStatus(execution.ID, "cancelled", "用户取消")
				return execution, ErrCancelled
			}
			e.logger.Error("node execution failed",
				zap.String("nodeId", nodeID),
				zap.Error(err))
			e.updateExecutionStatus(execution.ID, "failed", err.Error())
			return execution, err
		}

		// 节点产出里若包含 status=partial_success/skipped，记录到聚合状态
		if output != nil {
			if s, ok := output["status"].(string); ok && (s == "partial_success" || s == "partial") {
				partialNodes++
			}
		}

		nodeOutputs[nodeID] = output

		// 记录出边有效性：condition 节点按 conditionResult，其它节点默认有效。
		if nodeType == "condition" {
			outgoingActive[nodeID] = conditionPassed(output)
		} else {
			outgoingActive[nodeID] = true
		}

		e.logger.Info("node execution succeeded",
			zap.String("nodeId", nodeID),
			zap.Any("output", output),
			zap.Int("outputKeys", len(output)))
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
	e.setActiveNode(executionID, executor)
	defer e.clearActiveNode(executionID)

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

// topologicalSort 拓扑排序
func (e *Engine) topologicalSort(nodeList []map[string]interface{}, edgeList []map[string]interface{}) ([]map[string]interface{}, error) {
	// 构建邻接表和入度表
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

	// Kahn算法进行拓扑排序
	queue := []string{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	sorted := []map[string]interface{}{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nodeMap[current])

		for _, neighbor := range adjList[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// 检测循环依赖
	if len(sorted) != len(nodeList) {
		return nil, fmt.Errorf("workflow contains circular dependency")
	}

	return sorted, nil
}

// collectInputs 收集上游节点的输出
func (e *Engine) collectInputs(nodeID string, edgeList []map[string]interface{}, outputs map[string]map[string]interface{}) map[string]interface{} {
	input := make(map[string]interface{})

	for _, edge := range edgeList {
		if edge["target"].(string) == nodeID {
			sourceID := edge["source"].(string)
			if output, ok := outputs[sourceID]; ok {
				// 合并上游输出
				for k, v := range output {
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
